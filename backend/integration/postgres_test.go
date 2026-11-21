//go:build integration

// Package integration tests repositories and concurrency-sensitive flows
// against a real PostgreSQL (migrations applied):
//
//	GO_DOCKER_ARGS="--network lamma_lamma -e LAMMA_TEST_DATABASE_URL=postgres://lamma_app:...@postgres:5432/lamma" \
//	  scripts/go.sh go test -race -tags=integration ./integration/...
package integration

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("LAMMA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LAMMA_TEST_DATABASE_URL not set")
	}
	p, err := postgres.Connect(context.Background(), url, 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

type noopRooms struct{}

func (noopRooms) EnsureRoom(context.Context, string, int) error { return nil }

type signer struct{}

func (signer) Sign(g participant.Grant) (string, time.Time, error) { return "t", time.Now(), nil }

type notifier struct{}

func (notifier) Decided(context.Context, uuid.UUID, access.Status) {}
func (notifier) Waiting(context.Context, uuid.UUID, uuid.UUID)     {}

type env struct {
	tx       *postgres.TxManager
	meetings *meeting.PGStore
	svc      *meeting.Service
	parts    *participant.Service
	seats    *participant.Seats
	store    *participant.PGStore
}

func newEnv(t *testing.T) env {
	tx := postgres.NewTxManager(pool(t))
	ms, ps := meeting.NewPGStore(tx), participant.NewPGStore(tx)
	msvc := meeting.NewService(ms, nil, tx, nil, meeting.Config{})
	svc := participant.NewService(participant.Deps{
		Store: ps, Meetings: msvc, Notifier: notifier{}, Tx: tx,
		PasswordLimit: func(context.Context, string) error { return nil },
	})
	seats := participant.NewSeats(participant.SeatDeps{
		Store: ps, Meetings: msvc, Rooms: noopRooms{}, Signer: signer{}, Tx: tx, LeaseTTL: time.Minute,
	})
	return env{tx: tx, meetings: ms, svc: msvc, parts: svc, seats: seats, store: ps}
}

func (e env) createMeeting(t *testing.T, maxParticipants int, waitingRoom bool) meeting.Meeting {
	t.Helper()
	code, _ := meeting.NewCode()
	m := meeting.Meeting{ID: uuid.New(), Code: code, MaxParticipants: maxParticipants,
		WaitingRoom: waitingRoom, ExpiresAt: time.Now().Add(time.Hour)}
	if err := e.meetings.Insert(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (e env) join(t *testing.T, m meeting.Meeting, origin byte) access.Principal {
	t.Helper()
	res, err := e.parts.Join(context.Background(), participant.JoinInput{
		Code: m.Code, DisplayName: "Guest", Origin: []byte{origin},
	})
	if err != nil {
		t.Fatal(err)
	}
	return access.Principal{ParticipantID: res.Participant.ID, MeetingID: m.ID, Code: m.Code,
		Role: access.RoleGuest, Status: res.Participant.Status, MeetingActive: true, ExpiresAt: m.ExpiresAt}
}

// TestConcurrentSeatClaimsNeverExceedCapacity is the core concurrency
// guarantee of the seat model (PLAN.md §6): N parallel token requests for a
// meeting with capacity C yield exactly C seats.
func TestConcurrentSeatClaimsNeverExceedCapacity(t *testing.T) {
	e := newEnv(t)
	const capacity, guests = 3, 12
	m := e.createMeeting(t, capacity, false)
	principals := make([]access.Principal, guests)
	for i := range principals {
		principals[i] = e.join(t, m, byte(i)) // distinct origins: test capacity, not origin caps
	}
	var granted atomic.Int32
	var wg sync.WaitGroup
	for _, pr := range principals {
		wg.Go(func() {
			if _, err := e.seats.IssueToken(context.Background(), pr); err == nil {
				granted.Add(1)
			}
		})
	}
	wg.Wait()
	if got := granted.Load(); got != capacity {
		t.Fatalf("granted %d seats with capacity %d", got, capacity)
	}
}

func TestWaitingRoomCapPerOrigin(t *testing.T) {
	e := newEnv(t)
	m := e.createMeeting(t, 10, true)
	for range 3 {
		e.join(t, m, 7)
	}
	_, err := e.parts.Join(context.Background(), participant.JoinInput{Code: m.Code, DisplayName: "x", Origin: []byte{7}})
	if err == nil {
		t.Fatal("fourth waiter from one origin must be rejected")
	}
	e.join(t, m, 8) // other origins still welcome
}

func TestTransitionIsGuardedAndAudited(t *testing.T) {
	e := newEnv(t)
	m := e.createMeeting(t, 10, true)
	host, err := e.parts.RegisterHost(context.Background(), m.ID, "Host", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	guest := e.join(t, m, 2)
	ctx := context.Background()
	if err := e.parts.Admit(ctx, m.ID, host.ParticipantID, guest.ParticipantID); err != nil {
		t.Fatal(err)
	}
	if err := e.parts.Admit(ctx, m.ID, host.ParticipantID, guest.ParticipantID); err == nil {
		t.Fatal("admitting a non-waiting participant must fail")
	}
	other := e.createMeeting(t, 10, true)
	if err := e.parts.Admit(ctx, other.ID, host.ParticipantID, guest.ParticipantID); err == nil {
		t.Fatal("cross-meeting target must not be found (IDOR)")
	}
}

// TestReconcilerClaimsAreDisjoint checks SKIP LOCKED claiming across replicas.
func TestReconcilerClaimsAreDisjoint(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for range 20 {
		m := e.createMeeting(t, 2, false)
		if _, err := e.meetings.End(ctx, m.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	seen := map[uuid.UUID]int{}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			ids, err := e.meetings.ClaimRoomsToClose(ctx)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range ids {
				seen[id]++
			}
		})
	}
	wg.Wait()
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("meeting %s claimed %d times", id, n)
		}
	}
}

func TestExpiredMeetingsAreInvisible(t *testing.T) {
	e := newEnv(t)
	m := e.createMeeting(t, 2, false)
	_, err := e.tx.Conn(context.Background()).Exec(context.Background(),
		"UPDATE meetings SET expires_at = now() - interval '1 second' WHERE id = $1", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Lookup(context.Background(), m.Code); err == nil {
		t.Fatal("expired meeting must look like an unknown one before the worker runs")
	}
}
