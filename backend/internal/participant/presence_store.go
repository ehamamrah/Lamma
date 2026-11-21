package participant

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/participant/store"
)

// MarkJoined records presence; applied=false when no row matched (unknown
// identity or an out-of-order event).
func (s *PGStore) MarkJoined(ctx context.Context, id uuid.UUID, sid string, at time.Time) (uuid.UUID, access.Status, bool, error) {
	row, err := s.q(ctx).MarkJoined(ctx, store.MarkJoinedParams{ID: id, Sid: &sid, EventAt: &at})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false, nil
	}
	if err != nil {
		return uuid.Nil, "", false, err
	}
	return row.MeetingID, access.Status(row.Status), true, nil
}

// Exists reports whether a participant with id exists.
func (s *PGStore) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := s.q(ctx).GetParticipantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// MarkLeft clears presence for the matching LiveKit session.
func (s *PGStore) MarkLeft(ctx context.Context, id uuid.UUID, sid string, at time.Time) error {
	_, err := s.q(ctx).MarkLeft(ctx, store.MarkLeftParams{ID: id, Sid: &sid, EventAt: &at})
	return err
}

// MarkRoomEmpty clears presence for everyone in the meeting.
func (s *PGStore) MarkRoomEmpty(ctx context.Context, meetingID uuid.UUID) error {
	return s.q(ctx).MarkRoomEmpty(ctx, meetingID)
}

// Intruder is a connected participant who is no longer admitted.
type Intruder struct {
	ID        uuid.UUID
	MeetingID uuid.UUID
}

// ClaimIntruders claims connected-but-not-admitted participants (SKIP LOCKED).
func (s *PGStore) ClaimIntruders(ctx context.Context) ([]Intruder, error) {
	rows, err := s.q(ctx).ClaimIntruders(ctx)
	out := make([]Intruder, len(rows))
	for i, r := range rows {
		out[i] = Intruder{ID: r.ID, MeetingID: r.MeetingID}
	}
	return out, err
}

// MarkNotInRoom clears presence for one participant.
func (s *PGStore) MarkNotInRoom(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).MarkNotInRoom(ctx, id)
}

// AdmittedIDs lists admitted participant IDs of a meeting.
func (s *PGStore) AdmittedIDs(ctx context.Context, meetingID uuid.UUID) ([]uuid.UUID, error) {
	return s.q(ctx).ListAdmittedIDs(ctx, meetingID)
}

// InRoomIDs lists participants the DB believes are connected.
func (s *PGStore) InRoomIDs(ctx context.Context, meetingID uuid.UUID) ([]uuid.UUID, error) {
	return s.q(ctx).ListInRoomIDs(ctx, meetingID)
}

// CountInRoom counts connected participants across all meetings.
func (s *PGStore) CountInRoom(ctx context.Context) (int64, error) {
	return s.q(ctx).CountInRoom(ctx)
}
