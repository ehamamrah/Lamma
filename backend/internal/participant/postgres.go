package participant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/participant/store"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
)

// PGStore implements Store and PresenceStore, plus maintenance queries.
type PGStore struct {
	tx *postgres.TxManager
}

// NewPGStore creates a PGStore.
func NewPGStore(tx *postgres.TxManager) *PGStore { return &PGStore{tx: tx} }

func (s *PGStore) q(ctx context.Context) *store.Queries { return store.New(s.tx.Conn(ctx)) }

// LockMeeting locks the meeting row FOR UPDATE; call inside a transaction.
func (s *PGStore) LockMeeting(ctx context.Context, meetingID uuid.UUID) (MeetingLock, error) {
	row, err := s.q(ctx).LockMeeting(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MeetingLock{}, apperr.ErrNotFound
	}
	if err != nil {
		return MeetingLock{}, fmt.Errorf("lock meeting: %w", err)
	}
	return MeetingLock{MaxParticipants: int(row.MaxParticipants), Locked: row.Locked, WaitingRoom: row.WaitingRoom}, nil
}

// Insert creates a participant with a fresh UUIDv7.
func (s *PGStore) Insert(ctx context.Context, n NewParticipant) (Participant, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Participant{}, err
	}
	row, err := s.q(ctx).InsertParticipant(ctx, store.InsertParticipantParams{
		ID: id, MeetingID: n.MeetingID, DisplayName: n.DisplayName, Role: store.ParticipantRole(n.Role),
		Status: store.ParticipantStatus(n.Status), OriginHash: n.Origin, SessionHash: n.SessionHash,
	})
	return toParticipant(row, err)
}

// Get loads a participant scoped to its meeting (IDOR-safe).
func (s *PGStore) Get(ctx context.Context, meetingID, id uuid.UUID) (Participant, error) {
	row, err := s.q(ctx).GetParticipant(ctx, store.GetParticipantParams{ID: id, MeetingID: meetingID})
	return toParticipant(row, err)
}

// Transition applies a guarded status change and its audit event atomically.
func (s *PGStore) Transition(ctx context.Context, t Transition) (Participant, error) {
	var p Participant
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		row, err := s.q(ctx).SetParticipantStatus(ctx, store.SetParticipantStatusParams{
			ID: t.ID, MeetingID: t.MeetingID, Status: store.ParticipantStatus(t.To), FromStatuses: toStatuses(t.From),
		})
		if p, err = toParticipant(row, err); err != nil || t.Action == "" {
			return err
		}
		return s.audit(ctx, t.MeetingID, t.Actor, t.Action, &t.ID)
	})
	return p, err
}

func (s *PGStore) audit(ctx context.Context, meetingID uuid.UUID, actor *uuid.UUID, action string, target *uuid.UUID) error {
	return s.q(ctx).InsertParticipantEvent(ctx, store.InsertParticipantEventParams{
		MeetingID: meetingID, ActorID: actor, Action: action, TargetID: target,
	})
}

// Audit records a moderation event that changes no participant state.
func (s *PGStore) Audit(ctx context.Context, meetingID, actor uuid.UUID, action string, target uuid.UUID) error {
	return s.audit(ctx, meetingID, &actor, action, &target)
}

// RemoveGuest sets a guest to removed and audits it, returning the previous
// status; errNotFound if no removable guest matched.
func (s *PGStore) RemoveGuest(ctx context.Context, meetingID, actor, target uuid.UUID) (access.Status, error) {
	var prev access.Status
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		status, err := s.q(ctx).RemoveGuest(ctx, store.RemoveGuestParams{ID: target, MeetingID: meetingID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound
		}
		if err != nil {
			return fmt.Errorf("remove guest: %w", err)
		}
		prev = access.Status(status)
		return s.audit(ctx, meetingID, &actor, "remove", &target)
	})
	return prev, err
}

// HostID returns the meeting's host participant.
func (s *PGStore) HostID(ctx context.Context, meetingID uuid.UUID) (uuid.UUID, error) {
	return s.q(ctx).GetHostID(ctx, meetingID)
}

// CountAdmittedGuests counts admitted guest rows of a meeting.
func (s *PGStore) CountAdmittedGuests(ctx context.Context, meetingID uuid.UUID) (int64, error) {
	return s.q(ctx).CountAdmittedGuests(ctx, meetingID)
}

// CountWaiting counts fresh waiting entries for an origin and in total.
func (s *PGStore) CountWaiting(ctx context.Context, meetingID uuid.UUID, origin []byte) (int64, int64, error) {
	row, err := s.q(ctx).CountFreshWaiting(ctx, store.CountFreshWaitingParams{MeetingID: meetingID, OriginHash: origin})
	return row.ByOrigin, row.Total, err
}

// ListWaiting lists fresh waiting participants.
func (s *PGStore) ListWaiting(ctx context.Context, meetingID uuid.UUID) ([]Participant, error) {
	rows, err := s.q(ctx).ListFreshWaiting(ctx, meetingID)
	if err != nil {
		return nil, fmt.Errorf("list waiting: %w", err)
	}
	out := make([]Participant, 0, len(rows))
	for _, r := range rows {
		p, _ := toParticipant(r, nil)
		out = append(out, p)
	}
	return out, nil
}

// DenyAllWaiting denies every waiting participant and audits it once.
func (s *PGStore) DenyAllWaiting(ctx context.Context, meetingID, actor uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if ids, err = s.q(ctx).DenyAllWaiting(ctx, meetingID); err != nil || len(ids) == 0 {
			return err
		}
		return s.audit(ctx, meetingID, &actor, "deny_all", nil)
	})
	return ids, err
}

// CountSeats counts occupied guest seats, excluding the caller.
func (s *PGStore) CountSeats(ctx context.Context, meetingID, exclude uuid.UUID, origin []byte) (int64, int64, error) {
	row, err := s.q(ctx).CountSeats(ctx, store.CountSeatsParams{MeetingID: meetingID, ExcludeID: exclude, OriginHash: origin})
	return row.Total, row.UnconnectedByOrigin, err
}

// ClaimLease reserves the caller's seat for ttl.
func (s *PGStore) ClaimLease(ctx context.Context, id uuid.UUID, ttl time.Duration) (int, bool, error) {
	row, err := s.q(ctx).ClaimLease(ctx, store.ClaimLeaseParams{ID: id, Ttl: interval(ttl)})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, apperr.ErrForbidden
	}
	return int(row.LeaseCount), row.InRoom, err
}

// ReleaseLease frees the caller's seat reservation.
func (s *PGStore) ReleaseLease(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).ReleaseLease(ctx, id)
}

// AnyoneInRoom reports whether anyone is connected to the meeting's room.
func (s *PGStore) AnyoneInRoom(ctx context.Context, meetingID uuid.UUID) (bool, error) {
	return s.q(ctx).AnyoneInRoom(ctx, meetingID)
}

func toParticipant(row store.Participant, err error) (Participant, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Participant{}, errNotFound
	}
	if err != nil {
		return Participant{}, fmt.Errorf("participant query: %w", err)
	}
	return Participant{
		ID: row.ID, MeetingID: row.MeetingID, DisplayName: row.DisplayName,
		Role: access.Role(row.Role), Status: access.Status(row.Status), StatusAt: row.StatusAt,
		InRoom: row.InRoom, OriginHash: row.OriginHash,
	}, nil
}

// toStatuses encodes statuses as text: pgx cannot encode arrays of an
// unregistered enum type, so the query compares status::text.
func toStatuses(in []access.Status) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}

func interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}
