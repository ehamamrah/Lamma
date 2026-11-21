package participant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/meeting"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

type harness struct {
	svc       *Service
	seats     *Seats
	store     *memStore
	meetings  *fakeMeetings
	notifier  *fakeNotifier
	passwords *fakePasswords
	limitErr  error
	limitHits int
}

func newHarness(m meeting.Meeting, lock MeetingLock) *harness {
	h := &harness{store: newMemStore(lock), meetings: &fakeMeetings{m: m},
		notifier: &fakeNotifier{}, passwords: &fakePasswords{}}
	h.svc = NewService(Deps{
		Store: h.store, Meetings: h.meetings, Notifier: h.notifier, Passwords: h.passwords, Tx: inlineTx{},
		PasswordLimit: func(context.Context, string) error { h.limitHits++; return h.limitErr },
	})
	h.seats = h.newSeats(fakeRooms{})
	return h
}

func (h *harness) newSeats(rooms Rooms) *Seats {
	return NewSeats(SeatDeps{Store: h.store, Meetings: h.meetings, Rooms: rooms, Signer: fakeSigner{},
		Tx: inlineTx{}, LeaseTTL: time.Minute})
}

func testMeeting() meeting.Meeting {
	return meeting.Meeting{ID: uuid.New(), Code: "abcd-efgh-jkmn", MaxParticipants: 2, ExpiresAt: time.Now().Add(time.Hour)}
}

func codeOf(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	return ""
}

func TestJoinNew(t *testing.T) {
	locked := testMeeting()
	locked.Locked = true
	tests := []struct {
		name       string
		m          meeting.Meeting
		lock       MeetingLock
		in         JoinInput
		wantStatus access.Status
		wantCode   string
	}{
		{"admitted when no waiting room", testMeeting(), MeetingLock{}, JoinInput{DisplayName: "G"}, access.StatusAdmitted, ""},
		{"waiting when waiting room on", testMeeting(), MeetingLock{WaitingRoom: true}, JoinInput{DisplayName: "G"}, access.StatusWaiting, ""},
		{"locked meeting", locked, MeetingLock{Locked: true}, JoinInput{DisplayName: "G"}, "", "meeting_locked"},
		{"lock applied concurrently is respected", testMeeting(), MeetingLock{Locked: true}, JoinInput{DisplayName: "G"}, "", "meeting_locked"},
		{"invalid name", testMeeting(), MeetingLock{}, JoinInput{DisplayName: "‮"}, "", "invalid_display_name"},
		{"unknown code", testMeeting(), MeetingLock{}, JoinInput{DisplayName: "G", Code: "zzzz-zzzz-zzzz"}, "", "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.m, tt.lock)
			if tt.in.Code == "" {
				tt.in.Code = tt.m.Code
			}
			res, err := h.svc.Join(context.Background(), tt.in)
			if codeOf(err) != tt.wantCode || (err == nil && res.Participant.Status != tt.wantStatus) {
				t.Fatalf("got %v / %+v", err, res.Participant)
			}
			if err == nil && (res.Token == "" || res.Participant.Role != access.RoleGuest) {
				t.Fatal("new joins must get a guest session token")
			}
		})
	}
}

func TestJoinNotifiesHostOnlyForWaiting(t *testing.T) {
	h := newHarness(testMeeting(), MeetingLock{WaitingRoom: true})
	_, _ = h.svc.Join(context.Background(), JoinInput{Code: "abcd-efgh-jkmn", DisplayName: "G"})
	if h.notifier.waiting != 1 {
		t.Fatalf("host notifications = %d, want 1", h.notifier.waiting)
	}
}

func TestWaitingRoomCap(t *testing.T) {
	h := newHarness(testMeeting(), MeetingLock{WaitingRoom: true})
	for range maxWaitingPerOrigin {
		if _, err := h.svc.Join(context.Background(), JoinInput{Code: "abcd-efgh-jkmn", DisplayName: "G"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := h.svc.Join(context.Background(), JoinInput{Code: "abcd-efgh-jkmn", DisplayName: "G"})
	if codeOf(err) != "waiting_room_full" {
		t.Fatalf("got %v, want waiting_room_full", err)
	}
}

func TestPasswordCheck(t *testing.T) {
	m := testMeeting()
	m.PasswordHash = "hash:right-pass"
	tests := []struct {
		name          string
		password      string
		limitErr      error
		wantCode      string
		wantVerifies  int
		wantLimitHits int
	}{
		{"missing password does not consume budget", "", nil, "password_required", 0, 0},
		{"wrong password", "wrong-pass", nil, "invalid_password", 1, 1},
		{"right password", "right-pass", nil, "", 1, 1},
		{"limit checked before argon2", "right-pass", apperr.ErrRateLimited, "rate_limited", 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(m, MeetingLock{})
			h.limitErr = tt.limitErr
			_, err := h.svc.Join(context.Background(), JoinInput{Code: m.Code, DisplayName: "G", Password: tt.password})
			if codeOf(err) != tt.wantCode || h.passwords.calls != tt.wantVerifies || h.limitHits != tt.wantLimitHits {
				t.Fatalf("err=%v verifies=%d limits=%d", err, h.passwords.calls, h.limitHits)
			}
		})
	}
}

func principalFor(p Participant, m meeting.Meeting) access.Principal {
	return access.Principal{ParticipantID: p.ID, MeetingID: m.ID, Code: m.Code, Role: p.Role, Status: p.Status,
		MeetingActive: true, ExpiresAt: m.ExpiresAt}
}

func TestResume(t *testing.T) {
	tests := []struct {
		name     string
		role     access.Role
		status   access.Status
		lock     MeetingLock
		want     access.Status
		wantCode string
	}{
		{"admitted stays admitted", access.RoleGuest, access.StatusAdmitted, MeetingLock{Locked: true}, access.StatusAdmitted, ""},
		{"host always resumes", access.RoleHost, access.StatusAdmitted, MeetingLock{Locked: true}, access.StatusAdmitted, ""},
		{"removed cannot rejoin", access.RoleGuest, access.StatusRemoved, MeetingLock{}, "", "removed"},
		{"denied cannot rejoin", access.RoleGuest, access.StatusDenied, MeetingLock{}, "", "denied"},
		{"left guest re-evaluated: locked", access.RoleGuest, access.StatusLeft, MeetingLock{Locked: true}, "", "meeting_locked"},
		{"left guest re-evaluated: waiting room", access.RoleGuest, access.StatusLeft, MeetingLock{WaitingRoom: true}, access.StatusWaiting, ""},
		{"left guest re-admitted", access.RoleGuest, access.StatusLeft, MeetingLock{}, access.StatusAdmitted, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testMeeting()
			h := newHarness(m, tt.lock)
			p, _ := h.store.Insert(context.Background(), NewParticipant{MeetingID: m.ID, Role: tt.role, Status: tt.status})
			pr := principalFor(p, m)
			res, err := h.svc.Join(context.Background(), JoinInput{Code: m.Code, Session: &pr})
			if codeOf(err) != tt.wantCode || (err == nil && res.Participant.Status != tt.want) {
				t.Fatalf("got %v / %v", err, res.Participant.Status)
			}
			if err == nil && res.Token != "" {
				t.Fatal("resume must not mint a new session")
			}
		})
	}
}

func TestIssueToken(t *testing.T) {
	tests := []struct {
		name     string
		role     access.Role
		seats    int64
		pending  int64
		wantCode string
	}{
		{"guest with free seat", access.RoleGuest, 1, 0, ""},
		{"guest when full", access.RoleGuest, 2, 0, "meeting_full"},
		{"host exempt from full meeting", access.RoleHost, 2, 0, ""},
		{"too many unconnected from origin", access.RoleGuest, 0, maxUnconnectedPerOrigin, "too_many_pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testMeeting()
			h := newHarness(m, MeetingLock{})
			h.store.seats, h.store.pending = tt.seats, tt.pending
			p, _ := h.store.Insert(context.Background(), NewParticipant{MeetingID: m.ID, Role: tt.role, Status: access.StatusAdmitted})
			grant, err := h.seats.IssueToken(context.Background(), principalFor(p, m))
			if codeOf(err) != tt.wantCode || (err == nil && grant.Token == "") {
				t.Fatalf("got %v", err)
			}
			if err == nil && !h.meetings.roomMarked {
				t.Fatal("room must be ensured and recorded")
			}
		})
	}
}

func TestLeaseSquattingIsBounded(t *testing.T) {
	m := testMeeting()
	h := newHarness(m, MeetingLock{})
	p, _ := h.store.Insert(context.Background(), NewParticipant{MeetingID: m.ID, Role: access.RoleGuest, Status: access.StatusAdmitted})
	var err error
	for range maxLeasesWithoutConnecting + 1 {
		_, err = h.seats.IssueToken(context.Background(), principalFor(p, m))
	}
	if codeOf(err) != "too_many_pending" {
		t.Fatalf("renewing leases without connecting must be capped, got %v", err)
	}
}

func TestIssueTokenLiveKitDown(t *testing.T) {
	m := testMeeting()
	h := newHarness(m, MeetingLock{})
	seats := h.newSeats(fakeRooms{err: errors.New("connection refused")})
	p, _ := h.store.Insert(context.Background(), NewParticipant{MeetingID: m.ID, Role: access.RoleGuest, Status: access.StatusAdmitted})
	for range 5 {
		_, err := seats.IssueToken(context.Background(), principalFor(p, m))
		if appErr, _ := apperr.As(err); appErr == nil || appErr.Code != "livekit_unavailable" || appErr.RetryAfter == 0 {
			t.Fatalf("want livekit_unavailable with Retry-After, got %v", err)
		}
	}
	if h.store.leases[p.ID] != 0 {
		t.Fatal("a LiveKit outage must not consume lease attempts (review SP4-1)")
	}
}

func TestModerationStateChanges(t *testing.T) {
	m := testMeeting()
	h := newHarness(m, MeetingLock{})
	ctx := context.Background()
	host, _ := h.store.Insert(ctx, NewParticipant{MeetingID: m.ID, Role: access.RoleHost, Status: access.StatusAdmitted})
	waiter, _ := h.store.Insert(ctx, NewParticipant{MeetingID: m.ID, Role: access.RoleGuest, Status: access.StatusWaiting})

	if err := h.svc.Admit(ctx, m.ID, host.ID, waiter.ID); err != nil {
		t.Fatal(err)
	}
	if h.notifier.decided[waiter.ID] != access.StatusAdmitted {
		t.Fatal("admitted participant must be notified")
	}
	if err := h.svc.Admit(ctx, m.ID, host.ID, waiter.ID); codeOf(err) != "not_waiting" {
		t.Fatalf("admitting twice: got %v", err)
	}
	if err := h.svc.Remove(ctx, m.ID, host.ID, host.ID); codeOf(err) != "cannot_remove_host" {
		t.Fatalf("removing host: got %v", err)
	}
	if err := h.svc.Remove(ctx, m.ID, host.ID, waiter.ID); err != nil {
		t.Fatal(err)
	}
	if got := h.store.audits; len(got) != 2 || got[0] != "admit" || got[1] != "remove" {
		t.Fatalf("audit trail = %v", got)
	}
}

func TestHostNeverLockedOutByLeases(t *testing.T) {
	m := testMeeting()
	h := newHarness(m, MeetingLock{})
	p, _ := h.store.Insert(context.Background(), NewParticipant{MeetingID: m.ID, Role: access.RoleHost, Status: access.StatusAdmitted})
	for i := range 10 {
		if _, err := h.seats.IssueToken(context.Background(), principalFor(p, m)); err != nil {
			t.Fatalf("host token %d rejected: %v", i, err)
		}
	}
}

func TestAdmittedGuestRowsAreCapped(t *testing.T) {
	m := testMeeting()
	h := newHarness(m, MeetingLock{MaxParticipants: 2})
	h.store.admitted = admittedFactor * 2
	_, err := h.svc.Join(context.Background(), JoinInput{Code: m.Code, DisplayName: "G"})
	if codeOf(err) != "meeting_full" {
		t.Fatalf("got %v, want meeting_full", err)
	}
}
