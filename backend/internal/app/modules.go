package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/admission"
	"github.com/ehamamrah/lamma/backend/internal/chat"
	"github.com/ehamamrah/lamma/backend/internal/maintenance"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/moderation"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/password"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
	"github.com/ehamamrah/lamma/backend/internal/realtime"
)

// routes is everything the router mounts.
type routes struct {
	root     []func(chi.Router) // under /api/v1
	scoped   []func(chi.Router) // under /api/v1/meetings/{code}
	internal func(chi.Router)   // under /internal (never proxied publicly)
	resolver *access.Resolver
}

// container holds constructed components while wiring.
type container struct {
	deps
	tx         *postgres.TxManager
	cookies    access.Cookies
	lk         *realtime.Client
	hub        *admission.Hub
	meetStore  *meeting.PGStore
	partStore  *participant.PGStore
	meetings   *meeting.Service
	parts      *participant.Service
	chat       *chat.Service
	moderation *moderation.Service
}

// buildModules constructs every module and returns routes, workers and
// shutdown hooks.
func buildModules(d deps) (routes, []Worker, []func()) {
	c := newContainer(d)
	c.buildServices()
	return c.routes(), c.workers(), []func(){c.hub.Shutdown}
}

func newContainer(d deps) *container {
	tx := postgres.NewTxManager(d.infra.pool)
	return &container{
		deps:      d,
		tx:        tx,
		cookies:   access.Cookies{Secure: d.cfg.CookieSecure},
		lk:        realtime.NewClient(d.cfg.LiveKit, d.cfg.Breaker, d.infra.metrics),
		hub:       admission.NewHub(d.infra.redis, admission.Limits{PerIP: 3, Total: 2000}, d.infra.metrics.SSEStreams),
		meetStore: meeting.NewPGStore(tx),
		partStore: participant.NewPGStore(tx),
	}
}

func (c *container) buildServices() {
	hasher := password.NewHasher(c.cfg.Argon2MaxConcurrent)
	c.parts = participant.NewService(participant.Deps{
		Store: c.partStore, Meetings: c.meetStore, Rooms: c.lk,
		Signer:    realtime.NewSigner(c.cfg.LiveKit.APIKey, c.cfg.LiveKit.APISecret, c.cfg.LiveKit.TokenTTL),
		Notifier:  notifier{hub: c.hub, lk: c.lk},
		Passwords: hasher, Tx: c.tx, LeaseTTL: c.cfg.LiveKit.TokenTTL,
		PasswordLimit: participant.RateCheck(c.limiter.For(c.policies().password)),
	})
	c.meetings = meeting.NewService(c.meetStore, c.parts, c.tx, hasher, meeting.Config{
		DefaultTTL: c.cfg.Meeting.DefaultTTL, MaxTTL: c.cfg.Meeting.MaxTTL, MaxParticipants: c.cfg.Meeting.MaxParticipants,
	})
	c.chat = chat.NewService(c.lk, chat.NewRedisBuffer(c.infra.redis), chat.RateCheck(c.limiter.For(c.policies().chatMeeting)))
	c.moderation = moderation.NewService(c.parts, moderationMeetings{c.meetings, c.meetStore}, c.lk, c.chat, c.cfg.LiveKit.CallTimeout)
}

func (c *container) workers() []Worker {
	mw := maintenance.NewWorker(c.meetStore, maintenanceParticipants{c.partStore}, c.lk, maintenance.Gauges{
		MeetingsActive: c.infra.metrics.MeetingsActive, ParticipantsInRoom: c.infra.metrics.ParticipantsInRoom,
	}, c.cfg.Meeting.Retention)
	return []Worker{c.hub.Run, mw.Run}
}

// moderationMeetings combines the meeting service with room bookkeeping.
type moderationMeetings struct {
	*meeting.Service
	store *meeting.PGStore
}

func (m moderationMeetings) MarkRoomClosed(ctx context.Context, id uuid.UUID) error {
	return m.store.MarkRoomClosed(ctx, id)
}

// maintenanceParticipants adapts the participant store for the worker.
type maintenanceParticipants struct{ *participant.PGStore }

// notifier delivers waiting-room events: decisions via the Redis hub, and a
// "someone is waiting" signal to the room via LiveKit (hosts react to it).
type notifier struct {
	hub *admission.Hub
	lk  *realtime.Client
}

func (n notifier) Decided(ctx context.Context, id uuid.UUID, s access.Status) {
	n.hub.Decided(ctx, id, s)
}

func (n notifier) Waiting(ctx context.Context, meetingID uuid.UUID) {
	err := n.lk.SendData(ctx, meetingID.String(), "lamma.waiting", []byte(`{"type":"waiting"}`))
	if err != nil {
		slog.DebugContext(ctx, "waiting notification not delivered", slog.Any("error", err))
	}
}

// Middleware is an HTTP middleware.
type Middleware = func(http.Handler) http.Handler
