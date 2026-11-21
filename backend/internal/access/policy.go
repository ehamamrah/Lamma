// Package access owns identity and authorization: session tokens and
// cookies, the session → principal read model, and the permission policy.
// Roles come only from the server-side session row, never from clients.
package access

import (
	"time"

	"github.com/google/uuid"
)

// Role of a participant within one meeting.
type Role string

// Roles.
const (
	RoleHost  Role = "host"
	RoleGuest Role = "guest"
)

// Status of a participant within one meeting.
type Status string

// Participant statuses (mirrors the participant_status enum).
const (
	StatusWaiting  Status = "waiting"
	StatusAdmitted Status = "admitted"
	StatusDenied   Status = "denied"
	StatusRemoved  Status = "removed"
	StatusLeft     Status = "left"
)

// Principal is the authenticated caller for one request, resolved from the
// session cookie together with the meeting state needed to authorize it.
type Principal struct {
	ParticipantID uuid.UUID
	MeetingID     uuid.UUID
	Code          string
	DisplayName   string
	Role          Role
	Status        Status
	MeetingActive bool
	ExpiresAt     time.Time
	Locked        bool
	WaitingRoom   bool
}

// MeetingOpen reports whether the meeting is active and not expired at now.
func (p Principal) MeetingOpen(now time.Time) bool {
	return p.MeetingActive && now.Before(p.ExpiresAt)
}

// Action is a privileged operation subject to the policy.
type Action string

// Actions checked by Can.
const (
	ActionJoinMedia Action = "join_media" // obtain a LiveKit token
	ActionChat      Action = "chat"
	ActionModerate  Action = "moderate" // admit, deny, remove, mute, settings
	ActionEnd       Action = "end"
	ActionAwait     Action = "await" // waiting-room event stream
)

// policy lists which roles may perform each action. Every action except
// ActionAwait additionally requires the admitted status and an open meeting.
var policy = map[Action][]Role{
	ActionJoinMedia: {RoleHost, RoleGuest},
	ActionChat:      {RoleHost, RoleGuest},
	ActionModerate:  {RoleHost},
	ActionEnd:       {RoleHost},
	ActionAwait:     {RoleHost, RoleGuest},
}

// Can reports whether p may perform a at time now.
func Can(p Principal, a Action, now time.Time) bool {
	if !p.MeetingOpen(now) || !roleAllowed(a, p.Role) {
		return false
	}
	if a == ActionAwait {
		return p.Status == StatusWaiting || p.Status == StatusAdmitted
	}
	return p.Status == StatusAdmitted
}

func roleAllowed(a Action, r Role) bool {
	for _, allowed := range policy[a] {
		if allowed == r {
			return true
		}
	}
	return false
}
