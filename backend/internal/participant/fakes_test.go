package participant

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// memStore is an in-memory Store for service tests.
type memStore struct {
	mu      sync.Mutex
	lock    MeetingLock
	byID    map[uuid.UUID]*Participant
	leases  map[uuid.UUID]int
	pending int64
	audits  []string
}

func newMemStore(lock MeetingLock) *memStore {
	return &memStore{lock: lock, byID: map[uuid.UUID]*Participant{}, leases: map[uuid.UUID]int{}}
}

func (m *memStore) LockMeeting(context.Context, uuid.UUID) (MeetingLock, error) {
	return m.lock, nil
}

func (m *memStore) Insert(_ context.Context, n NewParticipant) (Participant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := Participant{ID: uuid.New(), MeetingID: n.MeetingID, DisplayName: n.DisplayName,
		Role: n.Role, Status: n.Status, StatusAt: time.Now(), OriginHash: n.Origin}
	m.byID[p.ID] = &p
	return p, nil
}

func (m *memStore) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (Participant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byID[id]; ok {
		return *p, nil
	}
	return Participant{}, errNotFound
}

func (m *memStore) Transition(_ context.Context, t Transition) (Participant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[t.ID]
	if !ok || !slices.Contains(t.From, p.Status) {
		return Participant{}, errNotFound
	}
	p.Status, p.StatusAt = t.To, time.Now()
	if t.Action != "" {
		m.audits = append(m.audits, t.Action)
	}
	return *p, nil
}

func (m *memStore) CountWaiting(context.Context, uuid.UUID, []byte) (int64, int64, error) {
	var n int64
	for _, p := range m.byID {
		if p.Status == access.StatusWaiting {
			n++
		}
	}
	return n, n, nil
}

func (m *memStore) ListWaiting(context.Context, uuid.UUID) ([]Participant, error) { return nil, nil }
func (m *memStore) DenyAllWaiting(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *memStore) CountUnconnected(context.Context, uuid.UUID, uuid.UUID, []byte) (int64, error) {
	return m.pending, nil
}
func (m *memStore) ClaimLease(_ context.Context, id uuid.UUID, _ time.Duration) (int, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leases[id]++
	return m.leases[id], false, nil
}
func (m *memStore) RemoveGuest(_ context.Context, _, _, target uuid.UUID) (access.Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[target]
	if !ok || p.Role != access.RoleGuest || p.Status == access.StatusRemoved || p.Status == access.StatusDenied {
		return "", errNotFound
	}
	prev := p.Status
	p.Status = access.StatusRemoved
	m.audits = append(m.audits, "remove")
	return prev, nil
}
func (m *memStore) HostID(context.Context, uuid.UUID) (uuid.UUID, error)  { return uuid.New(), nil }
func (m *memStore) ReleaseLease(context.Context, uuid.UUID) error         { return nil }
func (m *memStore) AnyoneInRoom(context.Context, uuid.UUID) (bool, error) { return false, nil }
func (m *memStore) Audit(_ context.Context, _, _ uuid.UUID, a string, _ uuid.UUID) error {
	m.audits = append(m.audits, a)
	return nil
}

type fakeMeetings struct {
	m          meeting.Meeting
	roomMarked bool
}

func (f *fakeMeetings) Lookup(_ context.Context, code string) (meeting.Meeting, error) {
	if code != f.m.Code {
		return meeting.Meeting{}, apperr.ErrNotFound
	}
	return f.m, nil
}
func (f *fakeMeetings) GetOpenByID(context.Context, uuid.UUID) (meeting.Meeting, error) {
	return f.m, nil
}
func (f *fakeMeetings) MarkRoomCreated(context.Context, uuid.UUID) error {
	f.roomMarked = true
	return nil
}

type fakeRooms struct{ err error }

func (f fakeRooms) EnsureRoom(context.Context, string) error { return f.err }

type fakeSigner struct{}

func (fakeSigner) Sign(g Grant) (string, time.Time, error) {
	return "jwt-for-" + g.Identity, time.Now().Add(time.Minute), nil
}

type fakeNotifier struct {
	mu      sync.Mutex
	waiting int
	decided map[uuid.UUID]access.Status
}

func (f *fakeNotifier) Decided(_ context.Context, id uuid.UUID, s access.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.decided == nil {
		f.decided = map[uuid.UUID]access.Status{}
	}
	f.decided[id] = s
}
func (f *fakeNotifier) Waiting(context.Context, uuid.UUID, uuid.UUID) { f.waiting++ }

type fakePasswords struct{ calls int }

func (f *fakePasswords) Verify(_ context.Context, encoded, pw string) (bool, error) {
	f.calls++
	return encoded == "hash:"+pw, nil
}

type inlineTx struct{}

func (inlineTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
