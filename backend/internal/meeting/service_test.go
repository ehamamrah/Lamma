package meeting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

type fakeStore struct {
	inserted   []Meeting
	collisions int
}

func (f *fakeStore) Insert(_ context.Context, m Meeting) error {
	if f.collisions > 0 {
		f.collisions--
		return ErrCodeTaken
	}
	f.inserted = append(f.inserted, m)
	return nil
}
func (f *fakeStore) GetOpenByCode(context.Context, string) (Meeting, error) {
	return Meeting{}, apperr.ErrNotFound
}
func (f *fakeStore) UpdateSettings(context.Context, uuid.UUID, uuid.UUID, Settings) (Meeting, error) {
	return Meeting{}, nil
}
func (f *fakeStore) End(context.Context, uuid.UUID, *uuid.UUID) (bool, error) { return true, nil }
func (f *fakeStore) GetOpenByID(context.Context, uuid.UUID) (Meeting, error)  { return Meeting{}, nil }
func (f *fakeStore) IsOpen(context.Context, uuid.UUID) (bool, error)          { return true, nil }
func (f *fakeStore) MarkRoomCreated(context.Context, uuid.UUID) error         { return nil }
func (f *fakeStore) ClearRoomCreated(context.Context, uuid.UUID) error        { return nil }
func (f *fakeStore) MarkRoomClosed(context.Context, uuid.UUID) error          { return nil }
func (f *fakeStore) ReopenRoomForClosing(context.Context, uuid.UUID) error    { return nil }

type fakeHosts struct{ calls int }

func (f *fakeHosts) RegisterHost(context.Context, uuid.UUID, string, []byte) (Host, error) {
	f.calls++
	return Host{ParticipantID: uuid.New(), Token: "tok"}, nil
}

type inlineTx struct{}

func (inlineTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeHasher struct{}

func (fakeHasher) Hash(_ context.Context, pw string) (string, error) { return "hashed:" + pw, nil }

func newService(store *fakeStore, hosts *fakeHosts) *Service {
	return NewService(store, hosts, inlineTx{}, fakeHasher{}, Config{
		DefaultTTL: 24 * time.Hour, MaxTTL: 48 * time.Hour, MaxParticipants: 50,
	})
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name     string
		in       CreateInput
		wantCode string
	}{
		{"ok minimal", CreateInput{DisplayName: "Ann"}, ""},
		{"ok with password", CreateInput{DisplayName: "Ann", Password: "12345678"}, ""},
		{"empty name", CreateInput{DisplayName: "  "}, "invalid_display_name"},
		{"name too long", CreateInput{DisplayName: strings.Repeat("a", 41)}, "invalid_display_name"},
		{"title too long", CreateInput{DisplayName: "A", Title: strings.Repeat("t", 121)}, "invalid_title"},
		{"short password", CreateInput{DisplayName: "A", Password: "1234567"}, "invalid_password"},
		{"too short ttl", CreateInput{DisplayName: "A", ExpiresIn: time.Minute}, "invalid_expiry"},
		{"too long ttl", CreateInput{DisplayName: "A", ExpiresIn: 72 * time.Hour}, "invalid_expiry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService(&fakeStore{}, &fakeHosts{}).Create(context.Background(), tt.in)
			if tt.wantCode == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if appErr, _ := apperr.As(err); tt.wantCode != "" && (appErr == nil || appErr.Code != tt.wantCode) {
				t.Fatalf("got %v, want %s", err, tt.wantCode)
			}
		})
	}
}

func TestCreateHashesPasswordAndRegistersHost(t *testing.T) {
	store, hosts := &fakeStore{}, &fakeHosts{}
	out, err := newService(store, hosts).Create(context.Background(), CreateInput{DisplayName: "Ann", Password: "12345678"})
	if err != nil {
		t.Fatal(err)
	}
	if store.inserted[0].PasswordHash != "hashed:12345678" || hosts.calls != 1 || out.Host.Token == "" {
		t.Fatalf("unexpected result: %+v %+v", store.inserted[0], out)
	}
	if !ValidCode(out.Meeting.Code) {
		t.Fatalf("invalid code %q", out.Meeting.Code)
	}
}

func TestCreateRetriesCodeCollisions(t *testing.T) {
	store := &fakeStore{collisions: 2}
	if _, err := newService(store, &fakeHosts{}).Create(context.Background(), CreateInput{DisplayName: "A"}); err != nil {
		t.Fatalf("two collisions must be retried: %v", err)
	}
	store = &fakeStore{collisions: 3}
	if _, err := newService(store, &fakeHosts{}).Create(context.Background(), CreateInput{DisplayName: "A"}); err == nil {
		t.Fatal("retries must be bounded")
	}
}

func TestCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		c, err := NewCode()
		if err != nil || !ValidCode(c) || seen[c] {
			t.Fatalf("bad or duplicate code %q (%v)", c, err)
		}
		seen[c] = true
	}
	for _, bad := range []string{"", "ABCD-EFGH-JKMN", "abcd-efgh-jkm", "abcd-efgh-jkmo", "abcd/efgh/jkmn", "../../etc"} {
		if ValidCode(bad) {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
