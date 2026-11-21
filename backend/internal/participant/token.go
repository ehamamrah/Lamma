package participant

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// TokenGrant is a signed LiveKit token for the caller.
type TokenGrant struct {
	Token     string
	ExpiresAt time.Time
}

// IssueToken claims a seat, makes sure the LiveKit room exists and signs a
// short-lived token whose lifetime equals the seat lease (PLAN.md §6, §9).
// The seat transaction commits before any LiveKit call.
func (s *Service) IssueToken(ctx context.Context, pr access.Principal) (TokenGrant, error) {
	m, err := s.Meetings.GetOpenByID(ctx, pr.MeetingID)
	if err != nil {
		return TokenGrant{}, err
	}
	if err := s.claimSeat(ctx, pr, m.MaxParticipants); err != nil {
		return TokenGrant{}, err
	}
	if err := s.ensureRoom(ctx, m); err != nil {
		return TokenGrant{}, err
	}
	token, exp, err := s.Signer.Sign(Grant{
		Room: m.ID.String(), Identity: pr.ParticipantID.String(), Name: pr.DisplayName, Role: string(pr.Role),
	})
	if err != nil {
		return TokenGrant{}, err
	}
	return TokenGrant{Token: token, ExpiresAt: exp}, nil
}

func (s *Service) claimSeat(ctx context.Context, pr access.Principal, maxSeats int) error {
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.Store.LockMeeting(ctx, pr.MeetingID); err != nil {
			return err
		}
		if pr.Role != access.RoleHost {
			if err := s.checkCapacity(ctx, pr, maxSeats); err != nil {
				return err
			}
		}
		leases, inRoom, err := s.Store.ClaimLease(ctx, pr.ParticipantID, s.LeaseTTL)
		if err != nil {
			return err
		}
		if !inRoom && leases > maxLeasesWithoutConnecting {
			return errTooManyPending
		}
		return nil
	})
}

// checkCapacity enforces max seats and the per-origin unconnected-seat cap.
// Hosts are exempt so they can always rejoin to moderate.
func (s *Service) checkCapacity(ctx context.Context, pr access.Principal, maxSeats int) error {
	cur, err := s.Store.Get(ctx, pr.MeetingID, pr.ParticipantID)
	if err != nil {
		return err
	}
	total, unconnected, err := s.Store.CountSeats(ctx, pr.MeetingID, pr.ParticipantID, cur.OriginHash)
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
func (s *Service) ensureRoom(ctx context.Context, m meeting.Meeting) error {
	if m.RoomCreated {
		busy, err := s.Store.AnyoneInRoom(ctx, m.ID)
		if err == nil && busy {
			return nil
		}
	}
	if err := s.Rooms.EnsureRoom(ctx, m.ID.String(), m.MaxParticipants+1); err != nil {
		slog.WarnContext(ctx, "ensure room failed", slog.Any("error", err))
		return liveKitError(err)
	}
	return s.Meetings.MarkRoomCreated(ctx, m.ID)
}

func liveKitError(err error) error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperr.ErrLiveKitDown.WithRetry(5 * time.Second)
}
