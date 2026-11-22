package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ehamamrah/lamma/backend/internal/meeting/store"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
)

// PGStore implements Store plus the room-bookkeeping and maintenance queries
// used by the realtime and maintenance modules.
type PGStore struct {
	tx *postgres.TxManager
}

// NewPGStore creates a PGStore.
func NewPGStore(tx *postgres.TxManager) *PGStore { return &PGStore{tx: tx} }

func (s *PGStore) q(ctx context.Context) *store.Queries { return store.New(s.tx.Conn(ctx)) }

// Insert stores m; ErrCodeTaken if the code already exists.
func (s *PGStore) Insert(ctx context.Context, m Meeting) error {
	n, err := s.q(ctx).InsertMeeting(ctx, store.InsertMeetingParams{
		ID: m.ID, Code: m.Code, Title: nullable(m.Title), PasswordHash: nullable(m.PasswordHash),
		WaitingRoom: m.WaitingRoom, ExpiresAt: m.ExpiresAt,
		MaxParticipants: int32(m.MaxParticipants), //nolint:gosec // validated 2..500
	})
	if err != nil {
		return fmt.Errorf("insert meeting: %w", err)
	}
	if n == 0 {
		return ErrCodeTaken
	}
	return nil
}

// GetOpenByCode returns an active, unexpired meeting.
func (s *PGStore) GetOpenByCode(ctx context.Context, code string) (Meeting, error) {
	row, err := s.q(ctx).GetOpenMeetingByCode(ctx, code)
	return toMeeting(row, err)
}

// GetOpenByID returns an active, unexpired meeting.
func (s *PGStore) GetOpenByID(ctx context.Context, id uuid.UUID) (Meeting, error) {
	row, err := s.q(ctx).GetOpenMeetingByID(ctx, id)
	return toMeeting(row, err)
}

// UpdateSettings updates settings and audits each change atomically.
func (s *PGStore) UpdateSettings(ctx context.Context, id, actor uuid.UUID, in Settings) (Meeting, error) {
	var m Meeting
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		row, err := s.q(ctx).UpdateMeetingSettings(ctx, store.UpdateMeetingSettingsParams{
			ID: id, Locked: optBool(in.Locked), WaitingRoom: optBool(in.WaitingRoom),
		})
		if m, err = toMeeting(row, err); err != nil {
			return err
		}
		return s.auditSettings(ctx, id, actor, in)
	})
	return m, err
}

func (s *PGStore) auditSettings(ctx context.Context, id, actor uuid.UUID, in Settings) error {
	for _, action := range settingsActions(in) {
		if err := s.audit(ctx, id, &actor, action); err != nil {
			return err
		}
	}
	return nil
}

func settingsActions(in Settings) []string {
	var out []string
	if in.Locked != nil {
		out = append(out, pick(*in.Locked, "lock", "unlock"))
	}
	if in.WaitingRoom != nil {
		out = append(out, pick(*in.WaitingRoom, "waiting_room_on", "waiting_room_off"))
	}
	return out
}

func pick(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// End marks the meeting ended (and audits it). False if already ended.
func (s *PGStore) End(ctx context.Context, id uuid.UUID, actor *uuid.UUID) (bool, error) {
	ended := false
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		_, err := s.q(ctx).EndMeeting(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("end meeting: %w", err)
		}
		ended = true
		return s.audit(ctx, id, actor, "end")
	})
	return ended, err
}

func (s *PGStore) audit(ctx context.Context, meetingID uuid.UUID, actor *uuid.UUID, action string) error {
	return s.q(ctx).InsertModerationEvent(ctx, store.InsertModerationEventParams{
		MeetingID: meetingID, ActorID: actor, Action: action,
	})
}

// MarkRoomCreated records that the LiveKit room exists.
func (s *PGStore) MarkRoomCreated(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).MarkRoomCreated(ctx, id)
}

// ClearRoomCreated records that the LiveKit room is gone.
func (s *PGStore) ClearRoomCreated(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).ClearRoomCreated(ctx, id)
}

// MarkRoomClosed records a confirmed DeleteRoom.
func (s *PGStore) MarkRoomClosed(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).MarkRoomClosed(ctx, id)
}

// ReopenRoomForClosing schedules an ended meeting's room for deletion again.
func (s *PGStore) ReopenRoomForClosing(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).ReopenRoomForClosing(ctx, id)
}

// IsOpen reports whether the meeting is active and unexpired.
func (s *PGStore) IsOpen(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.q(ctx).IsMeetingOpen(ctx, id)
}

// ExpireDue ends meetings past their expiry (batch, SKIP LOCKED).
func (s *PGStore) ExpireDue(ctx context.Context) ([]uuid.UUID, error) {
	return s.q(ctx).ExpireMeetings(ctx)
}

// ClaimRoomsToClose claims ended meetings whose room is still open.
func (s *PGStore) ClaimRoomsToClose(ctx context.Context) ([]uuid.UUID, error) {
	return s.q(ctx).ClaimRoomsToClose(ctx)
}

// ClaimRoomsToSweep claims live rooms due for an anti-entropy sweep.
func (s *PGStore) ClaimRoomsToSweep(ctx context.Context) ([]uuid.UUID, error) {
	return s.q(ctx).ClaimRoomsToSweep(ctx)
}

// PurgeEnded hard-deletes ended meetings older than retention.
func (s *PGStore) PurgeEnded(ctx context.Context, retention time.Duration) (int64, error) {
	return s.q(ctx).PurgeEndedMeetings(ctx, pgtype.Interval{Microseconds: retention.Microseconds(), Valid: true})
}

// CountActive counts open meetings (for the gauge).
func (s *PGStore) CountActive(ctx context.Context) (int64, error) {
	return s.q(ctx).CountActiveMeetings(ctx)
}

func toMeeting(row store.Meeting, err error) (Meeting, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Meeting{}, apperr.ErrNotFound
	}
	if err != nil {
		return Meeting{}, fmt.Errorf("load meeting: %w", err)
	}
	return Meeting{
		ID: row.ID, Code: row.Code, Title: deref(row.Title), PasswordHash: deref(row.PasswordHash),
		WaitingRoom: row.WaitingRoom, Locked: row.Locked, MaxParticipants: int(row.MaxParticipants),
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, RoomCreated: row.RoomCreatedAt != nil,
	}, nil
}

func optBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
