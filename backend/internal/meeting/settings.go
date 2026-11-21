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
