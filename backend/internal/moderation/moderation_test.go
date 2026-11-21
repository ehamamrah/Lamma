package moderation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

type fakeParts struct{ removed, admitted int }

func (f *fakeParts) Admit(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.admitted++
	return nil
}
func (f *fakeParts) Deny(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeParts) Remove(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.removed++
	return nil
}
func (f *fakeParts) DenyAllWaiting(context.Context, uuid.UUID, uuid.UUID) (int, error) { return 0, nil }
func (f *fakeParts) ListWaiting(context.Context, uuid.UUID) ([]participant.Participant, error) {
	return nil, nil
}
func (f *fakeParts) RecordMute(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }

type fakeMeetings struct{ closed bool }

func (f *fakeMeetings) UpdateSettings(context.Context, uuid.UUID, uuid.UUID, meeting.Settings) (meeting.Meeting, error) {
	return meeting.Meeting{}, nil
}
func (f *fakeMeetings) End(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
func (f *fakeMeetings) MarkRoomClosed(context.Context, uuid.UUID) error         { f.closed = true; return nil }

type fakeRoom struct{ err error }

func (f fakeRoom) RemoveParticipant(context.Context, string, string) error  { return f.err }
func (f fakeRoom) MuteSource(context.Context, string, string, string) error { return f.err }
func (f fakeRoom) DeleteRoom(context.Context, string) error                 { return f.err }

type fakeChat struct{}

func (fakeChat) Clear(context.Context, uuid.UUID) {}

func principal(role access.Role) access.Principal {
	return access.Principal{ParticipantID: uuid.New(), MeetingID: uuid.New(), Role: role,
		Status: access.StatusAdmitted, MeetingActive: true, ExpiresAt: time.Now().Add(time.Hour)}
}

func TestGuestsCannotModerate(t *testing.T) {
	parts := &fakeParts{}
	svc := NewService(parts, &fakeMeetings{}, fakeRoom{}, fakeChat{}, time.Second)
	guest := principal(access.RoleGuest)
	ctx := context.Background()
	checks := []error{
		svc.Admit(ctx, guest, uuid.New()),
		svc.Mute(ctx, guest, uuid.New(), "microphone"),
	}
	_, err := svc.Remove(ctx, guest, uuid.New())
	checks = append(checks, err)
	_, err = svc.End(ctx, guest)
	checks = append(checks, err)
	_, err = svc.UpdateSettings(ctx, guest, meeting.Settings{})
	checks = append(checks, err)
	for i, err := range checks {
		if !apperr.IsKind(err, apperr.KindForbidden) {
			t.Fatalf("check %d: want forbidden, got %v", i, err)
		}
	}
	if parts.removed+parts.admitted != 0 {
		t.Fatal("no state may change for unauthorized callers")
	}
}

func TestHostCannotRemoveSelf(t *testing.T) {
	host := principal(access.RoleHost)
	svc := NewService(&fakeParts{}, &fakeMeetings{}, fakeRoom{}, fakeChat{}, time.Second)
	if _, err := svc.Remove(context.Background(), host, host.ParticipantID); err == nil {
		t.Fatal("removing self must be rejected")
	}
}

func TestLiveKitFailureIsDeferredNotFailed(t *testing.T) {
	tests := []struct {
		name        string
		roomErr     error
		wantPending bool
	}{
		{"livekit ok", nil, false},
		{"livekit down", errors.New("unavailable"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meetings := &fakeMeetings{}
			svc := NewService(&fakeParts{}, meetings, fakeRoom{err: tt.roomErr}, fakeChat{}, 10*time.Millisecond)
			res, err := svc.End(context.Background(), principal(access.RoleHost))
			if err != nil || res.LiveKitPending != tt.wantPending || meetings.closed == tt.wantPending {
				t.Fatalf("res=%+v err=%v closed=%v", res, err, meetings.closed)
			}
		})
	}
}
