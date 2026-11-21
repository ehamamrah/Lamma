// Package participant owns participants and their sessions: joining and
// resuming, the waiting room, seat leases and LiveKit tokens, moderation
// state changes (with audit), and presence from LiveKit webhooks.
package participant

import (
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// Participant is one person's membership in one meeting.
type Participant struct {
	ID          uuid.UUID
	MeetingID   uuid.UUID
	DisplayName string
	Role        access.Role
	Status      access.Status
	StatusAt    time.Time
	InRoom      bool
	OriginHash  []byte
}

// waitingTTL is how long a waiting entry stays fresh (PLAN.md §9).
const waitingTTL = 10 * time.Minute

// FreshWaiting reports whether p is waiting and the entry has not gone stale.
func (p Participant) FreshWaiting(now time.Time) bool {
	return p.Status == access.StatusWaiting && now.Sub(p.StatusAt) < waitingTTL
}

// Limits on abuse of the waiting room and seats (PLAN.md §6, §9).
const (
	maxWaitingPerOrigin        = 3
	maxWaitingPerMeeting       = 50
	maxUnconnectedPerOrigin    = 3
	maxLeasesWithoutConnecting = 3
)

// Errors returned by the participant module.
var (
	errInvalidName      = apperr.New(apperr.KindInvalid, "invalid_display_name", "Name must be 1–40 characters.")
	errLocked           = apperr.New(apperr.KindForbidden, "meeting_locked", "The host has locked this meeting.")
	errRemoved          = apperr.New(apperr.KindForbidden, "removed", "You were removed from this meeting.")
	errDenied           = apperr.New(apperr.KindForbidden, "denied", "The host declined your request to join.")
	errPasswordRequired = apperr.New(apperr.KindUnauthorized, "password_required", "This meeting requires a password.")
	errInvalidPassword  = apperr.New(apperr.KindUnauthorized, "invalid_password", "Incorrect meeting password.")
	errFull             = apperr.New(apperr.KindConflict, "meeting_full", "This meeting is full.")
	errWaitingFull      = apperr.New(apperr.KindConflict, "waiting_room_full", "The waiting room is full. Try again later.")
	errTooManyPending   = apperr.New(apperr.KindRateLimited, "too_many_pending", "Too many unconnected joins from your network.").WithRetry(time.Minute)
	errNotWaiting       = apperr.New(apperr.KindConflict, "not_waiting", "That participant is not in the waiting room.")
	errCannotRemoveHost = apperr.New(apperr.KindForbidden, "cannot_remove_host", "The host cannot be removed.")
	errNotFound         = apperr.New(apperr.KindNotFound, "participant_not_found", "Participant not found.")
)
