package maintenance

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ehamamrah/lamma/backend/internal/participant"
	"github.com/ehamamrah/lamma/backend/internal/realtime"
)

type fakeMeetings struct {
	sweep       []uuid.UUID
	toClose     []uuid.UUID
	closed      []uuid.UUID
	roomCleared []uuid.UUID
}

func (f *fakeMeetings) ExpireDue(context.Context) ([]uuid.UUID, error)         { return nil, nil }
func (f *fakeMeetings) ClaimRoomsToClose(context.Context) ([]uuid.UUID, error) { return f.toClose, nil }
func (f *fakeMeetings) MarkRoomClosed(_ context.Context, id uuid.UUID) error {
	f.closed = append(f.closed, id)
	return nil
}
func (f *fakeMeetings) ClaimRoomsToSweep(context.Context) ([]uuid.UUID, error) { return f.sweep, nil }
func (f *fakeMeetings) ClearRoomCreated(_ context.Context, id uuid.UUID) error {
	f.roomCleared = append(f.roomCleared, id)
	return nil
}
func (f *fakeMeetings) PurgeEnded(context.Context, time.Duration) (int64, error) { return 0, nil }
func (f *fakeMeetings) CountActive(context.Context) (int64, error)               { return 3, nil }

type fakeParts struct {
	admitted  []uuid.UUID
	inRoom    []uuid.UUID
	notInRoom []uuid.UUID
	roomEmpty bool
}

func (f *fakeParts) ClaimIntruders(context.Context) ([]participant.Intruder, error) { return nil, nil }
func (f *fakeParts) MarkNotInRoom(_ context.Context, id uuid.UUID) error {
	f.notInRoom = append(f.notInRoom, id)
	return nil
}
func (f *fakeParts) AdmittedIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.admitted, nil
}
func (f *fakeParts) InRoomIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) { return f.inRoom, nil }
func (f *fakeParts) MarkRoomEmpty(context.Context, uuid.UUID) error            { f.roomEmpty = true; return nil }
func (f *fakeParts) CountInRoom(context.Context) (int64, error)                { return 7, nil }

type fakeLK struct {
	mu        sync.Mutex
	connected []string
	listErr   error
	deleteErr error
	removed   []string
}

func (f *fakeLK) DeleteRoom(context.Context, string) error { return f.deleteErr }
func (f *fakeLK) RemoveParticipant(_ context.Context, _, identity string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, identity)
	return nil
}
func (f *fakeLK) ListIdentities(context.Context, string) ([]string, error) {
	return f.connected, f.listErr
}

func gauges() Gauges {
	return Gauges{
		MeetingsActive:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "a"}),
		ParticipantsInRoom: prometheus.NewGauge(prometheus.GaugeOpts{Name: "b"}),
	}
}

func TestSweepEvictsUnadmittedAndClearsGhosts(t *testing.T) {
	room, admitted, intruder, ghost := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	m := &fakeMeetings{sweep: []uuid.UUID{room}}
	p := &fakeParts{admitted: []uuid.UUID{admitted, ghost}, inRoom: []uuid.UUID{admitted, ghost}}
	lk := &fakeLK{connected: []string{admitted.String(), intruder.String()}}
	NewWorker(m, p, lk, gauges(), time.Hour).Tick(context.Background())

	if !slices.Equal(lk.removed, []string{intruder.String()}) {
		t.Fatalf("removed = %v, want only the intruder", lk.removed)
	}
	if !slices.Equal(p.notInRoom, []uuid.UUID{ghost}) {
		t.Fatalf("ghost seats cleared = %v, want %v", p.notInRoom, ghost)
	}
}

func TestSweepHandlesVanishedRoom(t *testing.T) {
	room := uuid.New()
	m := &fakeMeetings{sweep: []uuid.UUID{room}}
	p := &fakeParts{}
	NewWorker(m, p, &fakeLK{listErr: realtime.ErrNotFound}, gauges(), time.Hour).Tick(context.Background())
	if !p.roomEmpty || !slices.Equal(m.roomCleared, []uuid.UUID{room}) {
		t.Fatal("a vanished room must clear presence and room_created_at")
	}
}

func TestCloseRoomsOnlyMarksConfirmedDeletes(t *testing.T) {
	ok, failing := uuid.New(), uuid.New()
	m := &fakeMeetings{toClose: []uuid.UUID{ok}}
	NewWorker(m, &fakeParts{}, &fakeLK{}, gauges(), time.Hour).Tick(context.Background())
	if !slices.Equal(m.closed, []uuid.UUID{ok}) {
		t.Fatalf("closed = %v", m.closed)
	}
	m = &fakeMeetings{toClose: []uuid.UUID{failing}}
	NewWorker(m, &fakeParts{}, &fakeLK{deleteErr: context.DeadlineExceeded}, gauges(), time.Hour).Tick(context.Background())
	if len(m.closed) != 0 {
		t.Fatal("failed deletes must stay pending for the next tick")
	}
}
