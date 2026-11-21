package participant

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/platform/text"
)

// JoinInput is a join request. Session is set when the caller already holds a
// valid session for this meeting (idempotent resume).
type JoinInput struct {
	Code        string
	DisplayName string
	Password    string
	Origin      []byte
	IPKey       string
	Session     *access.Principal
}

// JoinResult is the caller's participant; Token is set only for new sessions.
type JoinResult struct {
	Participant Participant
	Meeting     meeting.Meeting
	Token       string
}

// Join admits the caller, puts them in the waiting room, or resumes an
// existing session (PLAN.md §9).
func (s *Service) Join(ctx context.Context, in JoinInput) (JoinResult, error) {
	m, err := s.Meetings.Lookup(ctx, in.Code)
	if err != nil {
		return JoinResult{}, err
	}
	if in.Session != nil && in.Session.MeetingID == m.ID {
		return s.resume(ctx, m, *in.Session, in.Origin)
	}
	return s.joinNew(ctx, m, in)
}

func (s *Service) joinNew(ctx context.Context, m meeting.Meeting, in JoinInput) (JoinResult, error) {
	name, ok := text.Clean(in.DisplayName, 1, 40)
	if !ok {
		return JoinResult{}, errInvalidName
	}
	if m.Locked {
		return JoinResult{}, errLocked
	}
	if err := s.checkPassword(ctx, m, in); err != nil {
		return JoinResult{}, err
	}
	token, hash, err := access.NewSessionToken()
	if err != nil {
		return JoinResult{}, err
	}
	p, err := s.insertGuest(ctx, m.ID, NewParticipant{
		MeetingID: m.ID, DisplayName: name, Role: access.RoleGuest, Origin: in.Origin, SessionHash: hash,
	})
	if err != nil {
		return JoinResult{}, err
	}
	s.afterEntry(ctx, p)
	return JoinResult{Participant: p, Meeting: m, Token: token}, nil
}

// checkPassword consumes a rate-limit unit BEFORE running argon2, so parallel
// bursts cannot bypass the limit (PLAN.md §11).
func (s *Service) checkPassword(ctx context.Context, m meeting.Meeting, in JoinInput) error {
	if !m.RequiresPassword() {
		return nil
	}
	if in.Password == "" {
		return errPasswordRequired
	}
	if err := s.PasswordLimit(ctx, in.IPKey+"|"+m.ID.String()); err != nil {
		return err
	}
	ok, err := s.Passwords.Verify(ctx, m.PasswordHash, in.Password)
	if err != nil {
		return err
	}
	if !ok {
		return errInvalidPassword
	}
	return nil
}

// insertGuest decides admitted vs waiting under the meeting row lock, so lock
// and waiting-room changes made concurrently by the host are respected.
func (s *Service) insertGuest(ctx context.Context, meetingID uuid.UUID, n NewParticipant) (Participant, error) {
	var p Participant
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		status, err := s.entryStatus(ctx, meetingID, n.Origin)
		if err != nil {
			return err
		}
		n.Status = status
		p, err = s.Store.Insert(ctx, n)
		return err
	})
	if err != nil {
		return Participant{}, fmt.Errorf("join: %w", err)
	}
	return p, nil
}

// entryStatus must run inside a transaction: it locks the meeting row.
func (s *Service) entryStatus(ctx context.Context, meetingID uuid.UUID, origin []byte) (access.Status, error) {
	lock, err := s.Store.LockMeeting(ctx, meetingID)
	if err != nil {
		return "", err
	}
	if lock.Locked {
		return "", errLocked
	}
	if !lock.WaitingRoom {
		return access.StatusAdmitted, nil
	}
	return access.StatusWaiting, s.checkWaitingCaps(ctx, meetingID, origin)
}

func (s *Service) checkWaitingCaps(ctx context.Context, meetingID uuid.UUID, origin []byte) error {
	byOrigin, total, err := s.Store.CountWaiting(ctx, meetingID, origin)
	if err != nil {
		return err
	}
	if byOrigin >= maxWaitingPerOrigin || total >= maxWaitingPerMeeting {
		return errWaitingFull
	}
	return nil
}

// afterEntry runs post-commit side effects.
func (s *Service) afterEntry(ctx context.Context, p Participant) {
	if p.Status == access.StatusWaiting {
		s.Notifier.Waiting(context.WithoutCancel(ctx), p.MeetingID)
	}
}

// resume handles a join from a caller who already holds a session.
func (s *Service) resume(ctx context.Context, m meeting.Meeting, pr access.Principal, origin []byte) (JoinResult, error) {
	cur, err := s.Store.Get(ctx, m.ID, pr.ParticipantID)
	if err != nil {
		return JoinResult{}, err
	}
	switch {
	case cur.Status == access.StatusRemoved:
		return JoinResult{}, errRemoved
	case cur.Status == access.StatusDenied:
		return JoinResult{}, errDenied
	case cur.Role == access.RoleHost, cur.Status == access.StatusAdmitted, cur.FreshWaiting(s.now()):
		return JoinResult{Participant: cur, Meeting: m}, nil
	}
	p, err := s.reenter(ctx, cur, origin)
	if err != nil {
		return JoinResult{}, err
	}
	s.afterEntry(ctx, p)
	return JoinResult{Participant: p, Meeting: m}, nil
}

// reenter re-evaluates a guest who left (or whose waiting entry went stale)
// against the current lock and waiting-room settings and caps.
func (s *Service) reenter(ctx context.Context, cur Participant, origin []byte) (Participant, error) {
	var p Participant
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		status, err := s.entryStatus(ctx, cur.MeetingID, origin)
		if err != nil {
			return err
		}
		p, err = s.Store.Transition(ctx, Transition{
			MeetingID: cur.MeetingID, ID: cur.ID, To: status,
			From: []access.Status{access.StatusLeft, access.StatusWaiting},
		})
		return err
	})
	return p, err
}
