package participant

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// SeatStore is the persistence port for seat leases.
type SeatStore interface {
	LockMeeting(ctx context.Context, meetingID uuid.UUID) (MeetingLock, error)
	Get(ctx context.Context, meetingID, id uuid.UUID) (Participant, error)
	CountSeats(ctx context.Context, meetingID, exclude uuid.UUID, origin []byte) (total, unconnected int64, err error)
	ClaimLease(ctx context.Context, id uuid.UUID, ttl time.Duration) (leaseCount int, inRoom bool, err error)
	AnyoneInRoom(ctx context.Context, meetingID uuid.UUID) (bool, error)
}

// SeatMeetings is the subset of the meeting module seats need.
type SeatMeetings interface {
	GetOpenByID(ctx context.Context, id uuid.UUID) (meeting.Meeting, error)
	MarkRoomCreated(ctx context.Context, id uuid.UUID) error
}

// Rooms creates LiveKit rooms (idempotent).
type Rooms interface {
	EnsureRoom(ctx context.Context, room string, maxParticipants int) error
}

// Grant describes a LiveKit token to sign.
type Grant struct {
	Room     string
	Identity string
	Name     string
	Role     string
}

// TokenSigner signs LiveKit access tokens locally.
type TokenSigner interface {
	Sign(g Grant) (token string, expiresAt time.Time, err error)
}

// SeatDeps groups the seat dependencies.
type SeatDeps struct {
	Store    SeatStore
	Meetings SeatMeetings
	Rooms    Rooms
	Signer   TokenSigner
	Tx       Transactor
	LeaseTTL time.Duration
}

// Seats issues LiveKit tokens backed by seat leases (PLAN.md §6, ADR-013).
type Seats struct {
	d SeatDeps
}

// NewSeats creates Seats.
func NewSeats(d SeatDeps) *Seats { return &Seats{d: d} }

// TokenGrant is a signed LiveKit token for the caller.
type TokenGrant struct {
	Token     string
	ExpiresAt time.Time
}

// IssueToken ensures the room exists, then claims a seat lease and signs a
// token whose lifetime equals the lease. The room comes first so a LiveKit
// outage never consumes lease attempts; the seat transaction makes no network
// calls.
func (s *Seats) IssueToken(ctx context.Context, pr access.Principal) (TokenGrant, error) {
	m, err := s.d.Meetings.GetOpenByID(ctx, pr.MeetingID)
	if err != nil {
		return TokenGrant{}, err
	}
	if err := s.ensureRoom(ctx, m); err != nil {
		return TokenGrant{}, err
	}
	if err := s.claimSeat(ctx, pr, m.MaxParticipants); err != nil {
		return TokenGrant{}, err
	}
	token, exp, err := s.d.Signer.Sign(Grant{
		Room: m.ID.String(), Identity: pr.ParticipantID.String(), Name: pr.DisplayName, Role: string(pr.Role),
	})
	if err != nil {
		return TokenGrant{}, err
	}
	return TokenGrant{Token: token, ExpiresAt: exp}, nil
}

// claimSeat reserves the caller's seat. Hosts are exempt from capacity and
// from the unconnected-lease cap so they can always rejoin to moderate.
func (s *Seats) claimSeat(ctx context.Context, pr access.Principal, maxSeats int) error {
	return s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.d.Store.LockMeeting(ctx, pr.MeetingID); err != nil {
			return err
		}
		host := pr.Role == access.RoleHost
		if !host {
			if err := s.checkCapacity(ctx, pr, maxSeats); err != nil {
				return err
			}
		}
		leases, inRoom, err := s.d.Store.ClaimLease(ctx, pr.ParticipantID, s.d.LeaseTTL)
		if err == nil && !host && !inRoom && leases > maxLeasesWithoutConnecting {
			return errTooManyPending
		}
		return err
	})
}

// checkCapacity enforces max seats and the per-origin unconnected-seat cap.
func (s *Seats) checkCapacity(ctx context.Context, pr access.Principal, maxSeats int) error {
	cur, err := s.d.Store.Get(ctx, pr.MeetingID, pr.ParticipantID)
	if err != nil {
		return err
	}
	total, unconnected, err := s.d.Store.CountSeats(ctx, pr.MeetingID, pr.ParticipantID, cur.OriginHash)
	if err != nil {
		return err
	}
	if total >= int64(maxSeats) {
		return errFull
	}
	if !cur.InRoom && unconnected >= maxUnconnectedPerOrigin {
		return errTooManyPending
	}
	return nil
}

// ensureRoom calls CreateRoom unless the room is known to exist and is in use;
// a lost room_finished webhook can therefore never strand a meeting.
func (s *Seats) ensureRoom(ctx context.Context, m meeting.Meeting) error {
	if m.RoomCreated {
		busy, err := s.d.Store.AnyoneInRoom(ctx, m.ID)
		if err != nil {
			slog.DebugContext(ctx, "room occupancy check failed", slog.Any("error", err))
		}
		if busy {
			return nil
		}
	}
	if err := s.d.Rooms.EnsureRoom(ctx, m.ID.String(), m.MaxParticipants+1); err != nil {
		slog.WarnContext(ctx, "ensure room failed", slog.Any("error", err))
		return liveKitError(err)
	}
	return s.d.Meetings.MarkRoomCreated(ctx, m.ID)
}

func liveKitError(err error) error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperr.ErrLiveKitDown.WithRetry(5 * time.Second)
}
