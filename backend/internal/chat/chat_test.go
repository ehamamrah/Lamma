package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

type fakeBC struct{ sent int }

func (f *fakeBC) SendData(context.Context, string, string, []byte, ...string) error {
	f.sent++
	return nil
}

type fakeBuffer struct{ err error }

func (f fakeBuffer) Append(context.Context, uuid.UUID, []byte, time.Time) error { return f.err }
func (f fakeBuffer) Recent(context.Context, uuid.UUID) ([][]byte, error)        { return nil, f.err }
func (f fakeBuffer) Clear(context.Context, uuid.UUID) error                     { return f.err }

func pr() access.Principal {
	return access.Principal{ParticipantID: uuid.New(), MeetingID: uuid.New(), DisplayName: "Ann",
		ExpiresAt: time.Now().Add(time.Hour)}
}

func TestSend(t *testing.T) {
	limited := func(context.Context, string) error { return apperr.ErrRateLimited }
	open := func(context.Context, string) error { return nil }
	tests := []struct {
		name     string
		text     string
		limit    RateCheck
		buffer   error
		wantKind apperr.Kind
		wantSent int
	}{
		{"ok", "hello", open, nil, 0, 1},
		{"empty", "   ", open, nil, apperr.KindInvalid, 0},
		{"too long", strings.Repeat("a", 1001), open, nil, apperr.KindInvalid, 0},
		{"meeting limit", "hi", limited, nil, apperr.KindRateLimited, 0},
		{"buffer down still sends", "hi", open, errors.New("redis down"), 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := &fakeBC{}
			msg, err := NewService(bc, fakeBuffer{err: tt.buffer}, tt.limit).Send(context.Background(), pr(), tt.text)
			if tt.wantKind != 0 && !apperr.IsKind(err, tt.wantKind) || tt.wantKind == 0 && err != nil {
				t.Fatalf("err = %v", err)
			}
			if bc.sent != tt.wantSent || (err == nil && msg.DisplayName != "Ann") {
				t.Fatalf("sent=%d msg=%+v", bc.sent, msg)
			}
		})
	}
}

func TestRecentDegradesToEmpty(t *testing.T) {
	svc := NewService(&fakeBC{}, fakeBuffer{err: errors.New("down")}, nil)
	if got := svc.Recent(context.Background(), pr()); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
