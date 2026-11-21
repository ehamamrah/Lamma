// Package moderation orchestrates host actions. It owns no tables (ADR-012):
// it authorizes the host, asks the owning module to change state (which also
// writes the audit record), then applies the LiveKit side effect after the
// commit with a context the client cannot cancel.
package moderation

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// Participants is implemented by the participant module.
type Participants interface {
	Admit(ctx context.Context, meetingID, actor, target uuid.UUID) error
	Deny(ctx context.Context, meetingID, actor, target uuid.UUID) error
	Remove(ctx context.Context, meetingID, actor, target uuid.UUID) error
	DenyAllWaiting(ctx context.Context, meetingID, actor uuid.UUID) (int, error)
	ListWaiting(ctx context.Context, meetingID uuid.UUID) ([]participant.Participant, error)
	RecordMute(ctx context.Context, meetingID, actor, target uuid.UUID) error
}

// Meetings is implemented by the meeting module.
type Meetings interface {
	UpdateSettings(ctx context.Context, meetingID, actor uuid.UUID, s meeting.Settings) (meeting.Meeting, error)
	End(ctx context.Context, meetingID, actor uuid.UUID) (bool, error)
	MarkRoomClosed(ctx context.Context, id uuid.UUID) error
}

// Room is the LiveKit side of moderation.
type Room interface {
	RemoveParticipant(ctx context.Context, room, identity string) error
	MuteSource(ctx context.Context, room, identity, source string) error
	DeleteRoom(ctx context.Context, room string) error
}

// ChatHistory is cleared when a meeting ends.
type ChatHistory interface {
	Clear(ctx context.Context, meetingID uuid.UUID)
}

// Service implements host moderation.
type Service struct {
	parts    Participants
	meetings Meetings
	room     Room
	chat     ChatHistory
	timeout  time.Duration
}

// NewService creates the moderation service.
func NewService(p Participants, m Meetings, r Room, c ChatHistory, liveKitTimeout time.Duration) *Service {
	return &Service{parts: p, meetings: m, room: r, chat: c, timeout: liveKitTimeout}
}

// Result reports whether a LiveKit side effect was deferred to the reconciler.
type Result struct {
	LiveKitPending bool
}

// authorize re-checks the policy (defense in depth on top of middleware).
func authorize(pr access.Principal, a access.Action) error {
	if !access.Can(pr, a, time.Now()) {
		return apperr.ErrForbidden
	}
	return nil
}

// Admit admits a waiting participant.
func (s *Service) Admit(ctx context.Context, pr access.Principal, target uuid.UUID) error {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return err
	}
	return s.parts.Admit(ctx, pr.MeetingID, pr.ParticipantID, target)
}

// Deny rejects a waiting participant.
func (s *Service) Deny(ctx context.Context, pr access.Principal, target uuid.UUID) error {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return err
	}
	return s.parts.Deny(ctx, pr.MeetingID, pr.ParticipantID, target)
}

// DenyAllWaiting clears the waiting room.
func (s *Service) DenyAllWaiting(ctx context.Context, pr access.Principal) (int, error) {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return 0, err
	}
	return s.parts.DenyAllWaiting(ctx, pr.MeetingID, pr.ParticipantID)
}

// ListWaiting lists the waiting room.
func (s *Service) ListWaiting(ctx context.Context, pr access.Principal) ([]participant.Participant, error) {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return nil, err
	}
	return s.parts.ListWaiting(ctx, pr.MeetingID)
}

// Remove revokes the target's session, then disconnects them from LiveKit.
func (s *Service) Remove(ctx context.Context, pr access.Principal, target uuid.UUID) (Result, error) {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return Result{}, err
	}
	if target == pr.ParticipantID {
		return Result{}, apperr.New(apperr.KindInvalid, "cannot_remove_self", "You cannot remove yourself.")
	}
	if err := s.parts.Remove(ctx, pr.MeetingID, pr.ParticipantID, target); err != nil {
		return Result{}, err
	}
	err := s.afterCommit(ctx, func(ctx context.Context) error {
		return s.room.RemoveParticipant(ctx, pr.MeetingID.String(), target.String())
	})
	return Result{LiveKitPending: err != nil}, nil
}

// Mute server-mutes a source of the target. Hosts cannot unmute others.
func (s *Service) Mute(ctx context.Context, pr access.Principal, target uuid.UUID, source string) error {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return err
	}
	if err := s.parts.RecordMute(ctx, pr.MeetingID, pr.ParticipantID, target); err != nil {
		return err
	}
	return s.room.MuteSource(ctx, pr.MeetingID.String(), target.String(), source)
}

// UpdateSettings locks/unlocks the meeting or toggles the waiting room.
func (s *Service) UpdateSettings(ctx context.Context, pr access.Principal, in meeting.Settings) (meeting.Meeting, error) {
	if err := authorize(pr, access.ActionModerate); err != nil {
		return meeting.Meeting{}, err
	}
	return s.meetings.UpdateSettings(ctx, pr.MeetingID, pr.ParticipantID, in)
}

// End ends the meeting for everyone and closes the LiveKit room. If LiveKit
// is unavailable the reconciler closes the room later.
func (s *Service) End(ctx context.Context, pr access.Principal) (Result, error) {
	if err := authorize(pr, access.ActionEnd); err != nil {
		return Result{}, err
	}
	if _, err := s.meetings.End(ctx, pr.MeetingID, pr.ParticipantID); err != nil {
		return Result{}, err
	}
	s.chat.Clear(context.WithoutCancel(ctx), pr.MeetingID)
	err := s.afterCommit(ctx, func(ctx context.Context) error {
		if err := s.room.DeleteRoom(ctx, pr.MeetingID.String()); err != nil {
			return err
		}
		return s.meetings.MarkRoomClosed(ctx, pr.MeetingID)
	})
	return Result{LiveKitPending: err != nil}, nil
}

// afterCommit runs a side effect that a client disconnect must not skip.
func (s *Service) afterCommit(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*s.timeout)
	defer cancel()
	err := fn(ctx)
	if err != nil {
		slog.WarnContext(ctx, "livekit side effect deferred to reconciler", slog.Any("error", err))
	}
	return err
}
