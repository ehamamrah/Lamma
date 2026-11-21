package participant

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
)

// PresenceStore is the persistence port for webhook-driven presence.
type PresenceStore interface {
	MarkJoined(ctx context.Context, meetingID, id uuid.UUID, sid string, at time.Time) (status access.Status, applied bool, err error)
	Exists(ctx context.Context, meetingID, id uuid.UUID) (bool, error)
	MarkLeft(ctx context.Context, id uuid.UUID, sid string, at time.Time) error
	MarkRoomEmpty(ctx context.Context, meetingID uuid.UUID) error
}

// MeetingState reports whether a meeting is open.
type MeetingState interface {
	IsOpen(ctx context.Context, id uuid.UUID) (bool, error)
}

// Presence applies LiveKit participant events. It always records the fact
// that someone connected, then tells the caller whether that person must be
// removed (PLAN.md §9 Webhooks, ADR-014).
type Presence struct {
	store    PresenceStore
	meetings MeetingState
}

// NewPresence creates a Presence.
func NewPresence(store PresenceStore, meetings MeetingState) *Presence {
	return &Presence{store: store, meetings: meetings}
}

// Joined records a connection to meetingID's room and reports whether it is
// authorized. Identities unknown in that meeting are unauthorized; stale
// (out-of-order) events are ignored.
func (p *Presence) Joined(ctx context.Context, meetingID, identity uuid.UUID, sid string, at time.Time) (authorized bool, err error) {
	status, applied, err := p.store.MarkJoined(ctx, meetingID, identity, sid, at)
	if err != nil {
		return false, err
	}
	if !applied {
		exists, err := p.store.Exists(ctx, meetingID, identity)
		return exists, err // exists ⇒ stale event, nothing to do
	}
	if status != access.StatusAdmitted {
		return false, nil
	}
	return p.meetings.IsOpen(ctx, meetingID)
}

// Left records a disconnection (only for the current LiveKit session SID).
func (p *Presence) Left(ctx context.Context, identity uuid.UUID, sid string, at time.Time) error {
	return p.store.MarkLeft(ctx, identity, sid, at)
}

// RoomFinished marks everyone in the meeting as disconnected.
func (p *Presence) RoomFinished(ctx context.Context, meetingID uuid.UUID) error {
	return p.store.MarkRoomEmpty(ctx, meetingID)
}
