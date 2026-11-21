// Package maintenance runs the per-replica reconciliation loop (PLAN.md §10):
// expiry, closing rooms of ended meetings, evicting connected participants
// who are no longer admitted, an anti-entropy sweep against LiveKit, purging
// old meetings and refreshing gauges. All work is claimed from Postgres with
// SKIP LOCKED, so replicas never duplicate it and a crash loses nothing.
package maintenance

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/realtime"
)

// Meetings is the meeting-side maintenance store.
type Meetings interface {
	ExpireDue(ctx context.Context) ([]uuid.UUID, error)
	ClaimRoomsToClose(ctx context.Context) ([]uuid.UUID, error)
	MarkRoomClosed(ctx context.Context, id uuid.UUID) error
	ClaimRoomsToSweep(ctx context.Context) ([]uuid.UUID, error)
	ClearRoomCreated(ctx context.Context, id uuid.UUID) error
	PurgeEnded(ctx context.Context, retention time.Duration) (int64, error)
	CountActive(ctx context.Context) (int64, error)
}

// Participants is the participant-side maintenance store.
type Participants interface {
	ClaimIntruders(ctx context.Context) ([]participant.Intruder, error)
	MarkNotInRoom(ctx context.Context, id uuid.UUID) error
	AdmittedIDs(ctx context.Context, meetingID uuid.UUID) ([]uuid.UUID, error)
	InRoomIDs(ctx context.Context, meetingID uuid.UUID) ([]uuid.UUID, error)
	MarkRoomEmpty(ctx context.Context, meetingID uuid.UUID) error
	CountInRoom(ctx context.Context) (int64, error)
}

// LiveKit is the room API used for reconciliation.
type LiveKit interface {
	DeleteRoom(ctx context.Context, room string) error
	RemoveParticipant(ctx context.Context, room, identity string) error
	ListIdentities(ctx context.Context, room string) ([]string, error)
}

// Gauges are refreshed every tick.
type Gauges struct {
	MeetingsActive     prometheus.Gauge
	ParticipantsInRoom prometheus.Gauge
}

// Worker is the maintenance loop.
type Worker struct {
	meetings  Meetings
	parts     Participants
	lk        LiveKit
	gauges    Gauges
	retention time.Duration
	interval  time.Duration
}

// NewWorker creates the maintenance worker.
func NewWorker(m Meetings, p Participants, lk LiveKit, g Gauges, retention time.Duration) *Worker {
	return &Worker{meetings: m, parts: p, lk: lk, gauges: g, retention: retention, interval: 30 * time.Second}
}

// fanOutLimit bounds concurrent LiveKit calls per step.
const fanOutLimit = 8

// Run ticks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.Tick(ctx)
		}
	}
}

type step struct {
	name string
	run  func(context.Context) error
}

// Tick runs every step once; a failing step does not stop the others.
func (w *Worker) Tick(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, w.interval-5*time.Second)
	defer cancel()
	for _, s := range []step{
		{"expire", w.expire}, {"close_rooms", w.closeRooms}, {"evict_intruders", w.evictIntruders},
		{"sweep", w.sweep}, {"purge", w.purge}, {"gauges", w.refreshGauges},
	} {
		if err := s.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "maintenance step failed", slog.String("step", s.name), slog.Any("error", err))
		}
	}
}

func (w *Worker) expire(ctx context.Context) error {
	ids, err := w.meetings.ExpireDue(ctx)
	if len(ids) > 0 {
		slog.InfoContext(ctx, "meetings expired", slog.Int("count", len(ids)))
	}
	return err
}

func (w *Worker) closeRooms(ctx context.Context) error {
	ids, err := w.meetings.ClaimRoomsToClose(ctx)
	if err != nil {
		return err
	}
	return fanOut(ctx, ids, func(ctx context.Context, id uuid.UUID) error {
		if err := w.lk.DeleteRoom(ctx, id.String()); err != nil {
			return err
		}
		return w.meetings.MarkRoomClosed(ctx, id)
	})
}

func (w *Worker) evictIntruders(ctx context.Context) error {
	list, err := w.parts.ClaimIntruders(ctx)
	if err != nil {
		return err
	}
	return fanOut(ctx, list, func(ctx context.Context, in participant.Intruder) error {
		if err := w.lk.RemoveParticipant(ctx, in.MeetingID.String(), in.ID.String()); err != nil {
			return err
		}
		return w.parts.MarkNotInRoom(ctx, in.ID)
	})
}

func (w *Worker) purge(ctx context.Context) error {
	n, err := w.meetings.PurgeEnded(ctx, w.retention)
	if n > 0 {
		slog.InfoContext(ctx, "purged ended meetings", slog.Int64("count", n))
	}
	return err
}

func (w *Worker) refreshGauges(ctx context.Context) error {
	active, err := w.meetings.CountActive(ctx)
	if err != nil {
		return err
	}
	inRoom, err := w.parts.CountInRoom(ctx)
	if err != nil {
		return err
	}
	w.gauges.MeetingsActive.Set(float64(active))
	w.gauges.ParticipantsInRoom.Set(float64(inRoom))
	return nil
}

// fanOut applies fn to items with bounded concurrency, joining errors.
func fanOut[T any](ctx context.Context, items []T, fn func(context.Context, T) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	errs := make([]error, len(items))
	for i, item := range items {
		g.Go(func() error {
			errs[i] = fn(ctx, item)
			return nil
		})
	}
	_ = g.Wait()
	return errors.Join(errs...)
}

// isRoomGone reports a missing LiveKit room.
func isRoomGone(err error) bool { return errors.Is(err, realtime.ErrNotFound) }
