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
func (s *PGStore) MarkJoined(ctx context.Context, meetingID, id uuid.UUID, sid string, at time.Time) (access.Status, bool, error) {
	status, err := s.q(ctx).MarkJoined(ctx, store.MarkJoinedParams{ID: id, MeetingID: meetingID, Sid: &sid, EventAt: &at})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return access.Status(status), true, nil
}

// Exists reports whether participant id belongs to meetingID.
func (s *PGStore) Exists(ctx context.Context, meetingID, id uuid.UUID) (bool, error) {
	return s.q(ctx).ParticipantExists(ctx, store.ParticipantExistsParams{ID: id, MeetingID: meetingID})
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

// FilterAdmitted returns the admitted subset of candidates.
func (s *PGStore) FilterAdmitted(ctx context.Context, meetingID uuid.UUID, candidates []uuid.UUID) ([]uuid.UUID, error) {
	return s.q(ctx).FilterAdmitted(ctx, store.FilterAdmittedParams{MeetingID: meetingID, Candidates: candidates})
}

// InRoomIDs lists participants the DB believes are connected.
func (s *PGStore) InRoomIDs(ctx context.Context, meetingID uuid.UUID) ([]uuid.UUID, error) {
	return s.q(ctx).ListInRoomIDs(ctx, meetingID)
}

// CountInRoom counts connected participants across all meetings.
func (s *PGStore) CountInRoom(ctx context.Context) (int64, error) {
	return s.q(ctx).CountInRoom(ctx)
}
