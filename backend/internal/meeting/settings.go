package meeting

import (
	"context"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// Settings is a partial update; nil fields are unchanged.
type Settings struct {
	Locked      *bool
	WaitingRoom *bool
}

// Empty reports whether no field is set.
func (s Settings) Empty() bool { return s.Locked == nil && s.WaitingRoom == nil }

// UpdateSettings applies host settings and records an audit event in the same
// statement. Authorization is the caller's (moderation) responsibility.
func (s *Service) UpdateSettings(ctx context.Context, meetingID, actor uuid.UUID, in Settings) (Meeting, error) {
	if in.Empty() {
		return Meeting{}, apperr.New(apperr.KindInvalid, "empty_update", "Nothing to update.")
	}
	return s.store.UpdateSettings(ctx, meetingID, actor, in)
}

// End ends the meeting for everyone. It reports false if it had already ended.
func (s *Service) End(ctx context.Context, meetingID, actor uuid.UUID) (bool, error) {
	return s.store.End(ctx, meetingID, &actor)
}

func errNotFound() error { return apperr.ErrNotFound }

// GetOpenByID returns an active, unexpired meeting.
func (s *Service) GetOpenByID(ctx context.Context, id uuid.UUID) (Meeting, error) {
	return s.store.GetOpenByID(ctx, id)
}

// IsOpen reports whether the meeting is active and unexpired.
func (s *Service) IsOpen(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.store.IsOpen(ctx, id)
}

// MarkRoomCreated records that the LiveKit room exists.
func (s *Service) MarkRoomCreated(ctx context.Context, id uuid.UUID) error {
	return s.store.MarkRoomCreated(ctx, id)
}

// ClearRoomCreated records that the LiveKit room is gone.
func (s *Service) ClearRoomCreated(ctx context.Context, id uuid.UUID) error {
	return s.store.ClearRoomCreated(ctx, id)
}

// MarkRoomClosed records a confirmed room deletion after the meeting ended.
func (s *Service) MarkRoomClosed(ctx context.Context, id uuid.UUID) error {
	return s.store.MarkRoomClosed(ctx, id)
}

// ReopenRoomForClosing schedules an ended meeting's room for deletion again.
func (s *Service) ReopenRoomForClosing(ctx context.Context, id uuid.UUID) error {
	return s.store.ReopenRoomForClosing(ctx, id)
}
