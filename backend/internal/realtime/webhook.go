package realtime

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
)

// PresenceSink records participant presence (participant module).
type PresenceSink interface {
	Joined(ctx context.Context, meetingID, identity uuid.UUID, sid string, at time.Time) (authorized bool, err error)
	Left(ctx context.Context, identity uuid.UUID, sid string, at time.Time) error
	RoomFinished(ctx context.Context, meetingID uuid.UUID) error
}

// RoomSink records room lifecycle (meeting module).
type RoomSink interface {
	IsOpen(ctx context.Context, id uuid.UUID) (bool, error)
	ClearRoomCreated(ctx context.Context, id uuid.UUID) error
	ReopenRoomForClosing(ctx context.Context, id uuid.UUID) error
}

// remover is the subset of Client used to evict intruders.
type remover interface {
	RemoveParticipant(ctx context.Context, room, identity string) error
	DeleteRoom(ctx context.Context, room string) error
}

// WebhookHandler verifies and applies LiveKit webhooks. It enforces that only
// admitted participants of open meetings stay connected (ADR-014).
type WebhookHandler struct {
	keys     auth.KeyProvider
	presence PresenceSink
	rooms    RoomSink
	lk       remover
}

// NewWebhookHandler creates a WebhookHandler.
func NewWebhookHandler(key, secret string, p PresenceSink, r RoomSink, lk remover) *WebhookHandler {
	return &WebhookHandler{keys: auth.NewSimpleKeyProvider(key, secret), presence: p, rooms: r, lk: lk}
}

const maxWebhookBytes = 64 << 10

// ServeHTTP verifies the signature (and body hash), then dispatches.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBytes)
	ev, err := webhook.ReceiveWebhookEvent(r, h.keys)
	if err != nil {
		slog.WarnContext(r.Context(), "webhook rejected", slog.Any("error", err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := h.dispatch(r.Context(), ev); err != nil {
		slog.ErrorContext(r.Context(), "webhook failed", slog.String("event", ev.GetEvent()), slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) dispatch(ctx context.Context, ev *livekit.WebhookEvent) error {
	at := time.Unix(ev.GetCreatedAt(), 0)
	room := ev.GetRoom().GetName()
	switch ev.GetEvent() {
	case webhook.EventParticipantJoined:
		return h.joined(ctx, room, ev.GetParticipant(), at)
	case webhook.EventParticipantLeft:
		return h.left(ctx, ev.GetParticipant(), at)
	case webhook.EventRoomStarted:
		return h.roomStarted(ctx, room)
	case webhook.EventRoomFinished:
		return h.roomFinished(ctx, room)
	}
	return nil
}

func (h *WebhookHandler) joined(ctx context.Context, room string, p *livekit.ParticipantInfo, at time.Time) error {
	authorized, err := h.authorize(ctx, room, p, at)
	if err != nil {
		return err
	}
	if authorized {
		return nil
	}
	slog.WarnContext(ctx, "evicting unauthorized participant", slog.String("meeting_id", room))
	return h.lk.RemoveParticipant(context.WithoutCancel(ctx), room, p.GetIdentity())
}

// authorize records presence; identities or rooms that are not ours are
// unauthorized (this deployment runs no egress/agents, so every legitimate
// identity is a participant UUID).
func (h *WebhookHandler) authorize(ctx context.Context, room string, p *livekit.ParticipantInfo, at time.Time) (bool, error) {
	meetingID, errRoom := uuid.Parse(room)
	id, errID := uuid.Parse(p.GetIdentity())
	if errRoom != nil || errID != nil {
		return false, nil
	}
	return h.presence.Joined(ctx, meetingID, id, p.GetSid(), at)
}

func (h *WebhookHandler) left(ctx context.Context, p *livekit.ParticipantInfo, at time.Time) error {
	id, err := uuid.Parse(p.GetIdentity())
	if err != nil {
		return nil // not one of ours; nothing recorded
	}
	return h.presence.Left(ctx, id, p.GetSid(), at)
}

// roomStarted closes rooms for meetings that are no longer open (e.g. a
// CreateRoom raced with End).
func (h *WebhookHandler) roomStarted(ctx context.Context, room string) error {
	id, err := uuid.Parse(room)
	if err != nil {
		return h.lk.DeleteRoom(context.WithoutCancel(ctx), room)
	}
	open, err := h.rooms.IsOpen(ctx, id)
	if err != nil || open {
		return err
	}
	if err := h.rooms.ReopenRoomForClosing(ctx, id); err != nil {
		return err
	}
	return h.lk.DeleteRoom(context.WithoutCancel(ctx), room)
}

func (h *WebhookHandler) roomFinished(ctx context.Context, room string) error {
	id, err := uuid.Parse(room)
	if err != nil {
		return nil
	}
	if err := h.presence.RoomFinished(ctx, id); err != nil {
		return err
	}
	return h.rooms.ClearRoomCreated(ctx, id)
}
