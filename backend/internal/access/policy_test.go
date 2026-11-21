package access

import (
	"testing"
	"time"
)

func TestCanMatrix(t *testing.T) {
	now := time.Now()
	open := func(role Role, status Status) Principal {
		return Principal{Role: role, Status: status, MeetingActive: true, ExpiresAt: now.Add(time.Hour)}
	}
	expired := open(RoleHost, StatusAdmitted)
	expired.ExpiresAt = now.Add(-time.Second)
	ended := open(RoleHost, StatusAdmitted)
	ended.MeetingActive = false

	tests := []struct {
		name   string
		p      Principal
		action Action
		want   bool
	}{
		{"host moderates", open(RoleHost, StatusAdmitted), ActionModerate, true},
		{"host ends", open(RoleHost, StatusAdmitted), ActionEnd, true},
		{"guest cannot moderate", open(RoleGuest, StatusAdmitted), ActionModerate, false},
		{"guest cannot end", open(RoleGuest, StatusAdmitted), ActionEnd, false},
		{"guest chats", open(RoleGuest, StatusAdmitted), ActionChat, true},
		{"guest joins media", open(RoleGuest, StatusAdmitted), ActionJoinMedia, true},
		{"waiting guest cannot join media", open(RoleGuest, StatusWaiting), ActionJoinMedia, false},
		{"waiting guest cannot chat", open(RoleGuest, StatusWaiting), ActionChat, false},
		{"waiting guest awaits", open(RoleGuest, StatusWaiting), ActionAwait, true},
		{"removed guest cannot await", open(RoleGuest, StatusRemoved), ActionAwait, false},
		{"removed guest cannot join media", open(RoleGuest, StatusRemoved), ActionJoinMedia, false},
		{"denied guest cannot join media", open(RoleGuest, StatusDenied), ActionJoinMedia, false},
		{"left guest must rejoin first", open(RoleGuest, StatusLeft), ActionJoinMedia, false},
		{"expired meeting blocks host", expired, ActionModerate, false},
		{"ended meeting blocks host", ended, ActionJoinMedia, false},
		{"unknown action denied", open(RoleHost, StatusAdmitted), Action("delete_everything"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Can(tt.p, tt.action, now); got != tt.want {
				t.Fatalf("Can(%s/%s, %s) = %v, want %v", tt.p.Role, tt.p.Status, tt.action, got, tt.want)
			}
		})
	}
}

func TestSessionTokens(t *testing.T) {
	tok1, hash1, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	tok2, _, _ := NewSessionToken()
	if tok1 == tok2 || len(tok1) < 43 {
		t.Fatalf("tokens must be unique and 256-bit: %q %q", tok1, tok2)
	}
	if string(HashToken(tok1)) != string(hash1) {
		t.Fatal("hash must be reproducible from the raw token")
	}
}
