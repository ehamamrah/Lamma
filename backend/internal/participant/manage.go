package participant

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
)

// RegisterHost creates the meeting creator's participant; it runs inside the
// meeting-creation transaction (implements meeting.HostRegistrar).
func (s *Service) RegisterHost(ctx context.Context, meetingID uuid.UUID, name string, origin []byte) (meeting.Host, error) {
	token, hash, err := access.NewSessionToken()
	if err != nil {
		return meeting.Host{}, err
	}
	p, err := s.Store.Insert(ctx, NewParticipant{
		MeetingID: meetingID, DisplayName: name, Role: access.RoleHost,
		Status: access.StatusAdmitted, Origin: origin, SessionHash: hash,
	})
	if err != nil {
		return meeting.Host{}, err
	}
	return meeting.Host{ParticipantID: p.ID, Token: token}, nil
}

// Admit moves a waiting participant into the meeting. Capacity is enforced
// later, when the participant requests a token.
func (s *Service) Admit(ctx context.Context, meetingID, actor, target uuid.UUID) error {
	return s.decide(ctx, meetingID, actor, target, access.StatusAdmitted, "admit")
}

// Deny rejects a waiting participant.
func (s *Service) Deny(ctx context.Context, meetingID, actor, target uuid.UUID) error {
	return s.decide(ctx, meetingID, actor, target, access.StatusDenied, "deny")
}

func (s *Service) decide(ctx context.Context, meetingID, actor, target uuid.UUID, to access.Status, action string) error {
	_, err := s.Store.Transition(ctx, Transition{
		MeetingID: meetingID, ID: target, To: to,
		From: []access.Status{access.StatusWaiting}, Actor: &actor, Action: action,
	})
	if errors.Is(err, errNotFound) {
		return errNotWaiting
	}
	if err != nil {
		return err
	}
	s.Notifier.Decided(context.WithoutCancel(ctx), target, to)
	return nil
}

// DenyAllWaiting clears the waiting room in one action.
func (s *Service) DenyAllWaiting(ctx context.Context, meetingID, actor uuid.UUID) (int, error) {
	ids, err := s.Store.DenyAllWaiting(ctx, meetingID, actor)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		s.Notifier.Decided(context.WithoutCancel(ctx), id, access.StatusDenied)
	}
	return len(ids), nil
}

// Remove revokes a guest's session. The caller (moderation) then removes the
// participant from LiveKit; the webhook guard and reconciler cover failures.
func (s *Service) Remove(ctx context.Context, meetingID, actor, target uuid.UUID) error {
	cur, err := s.Store.Get(ctx, meetingID, target)
	if err != nil {
		return err
	}
	if cur.Role == access.RoleHost {
		return errCannotRemoveHost
	}
	_, err = s.Store.Transition(ctx, Transition{
		MeetingID: meetingID, ID: target, To: access.StatusRemoved, Actor: &actor, Action: "remove",
		From: []access.Status{access.StatusAdmitted, access.StatusWaiting, access.StatusLeft},
	})
	if err == nil && cur.Status == access.StatusWaiting {
		s.Notifier.Decided(context.WithoutCancel(ctx), target, access.StatusDenied)
	}
	return err
}

// Leave releases the caller's seat. The session is kept so the caller can
// rejoin; hosts keep their admitted status (PLAN.md A9).
func (s *Service) Leave(ctx context.Context, pr access.Principal) error {
	if err := s.Store.ReleaseLease(ctx, pr.ParticipantID); err != nil {
		return err
	}
	if pr.Role == access.RoleHost {
		return nil
	}
	_, err := s.Store.Transition(ctx, Transition{
		MeetingID: pr.MeetingID, ID: pr.ParticipantID, To: access.StatusLeft,
		From: []access.Status{access.StatusAdmitted, access.StatusWaiting},
	})
	if errors.Is(err, errNotFound) {
		return nil // already left / removed: leaving is idempotent
	}
	return err
}

// ListWaiting returns fresh waiting participants (max 100).
func (s *Service) ListWaiting(ctx context.Context, meetingID uuid.UUID) ([]Participant, error) {
	return s.Store.ListWaiting(ctx, meetingID)
}

// RecordMute audits a host mute action against a participant of the meeting.
func (s *Service) RecordMute(ctx context.Context, meetingID, actor, target uuid.UUID) error {
	if _, err := s.Store.Get(ctx, meetingID, target); err != nil {
		return err
	}
	return s.Store.Audit(ctx, meetingID, actor, "mute", target)
}

// CurrentStatus returns the caller's status (waiting-room stream).
func (s *Service) CurrentStatus(ctx context.Context, pr access.Principal) (access.Status, error) {
	p, err := s.Store.Get(ctx, pr.MeetingID, pr.ParticipantID)
	return p.Status, err
}

// Current returns the caller's participant row.
func (s *Service) Current(ctx context.Context, pr access.Principal) (Participant, error) {
	return s.Store.Get(ctx, pr.MeetingID, pr.ParticipantID)
}
