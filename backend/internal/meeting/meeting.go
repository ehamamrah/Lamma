// Package meeting owns the meeting lifecycle: creation, public lookup,
// settings (lock, waiting room), ending, and LiveKit room bookkeeping.
package meeting

import (
	"crypto/rand"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

// Meeting is the meeting aggregate. PasswordHash never leaves the server.
type Meeting struct {
	ID              uuid.UUID
	Code            string
	Title           string
	PasswordHash    string
	WaitingRoom     bool
	Locked          bool
	MaxParticipants int
	CreatedAt       time.Time
	ExpiresAt       time.Time
	RoomCreated     bool
}

// RequiresPassword reports whether joining needs a password.
func (m Meeting) RequiresPassword() bool { return m.PasswordHash != "" }

// codeAlphabet is Crockford base32 in lower case (no i, l, o, u): 32 symbols,
// so masking a random byte with 31 is unbiased. 12 symbols = 60 bits.
const codeAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

var codePattern = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{4}-[0-9a-hjkmnp-tv-z]{4}-[0-9a-hjkmnp-tv-z]{4}$`)

// NewCode returns a cryptographically random code formatted xxxx-xxxx-xxxx.
func NewCode() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 0, 14)
	for i, b := range raw {
		if i == 4 || i == 8 {
			out = append(out, '-')
		}
		out = append(out, codeAlphabet[b&31])
	}
	return string(out), nil
}

// ValidCode reports whether s is a canonical meeting code. Checking the
// format first avoids database lookups for garbage input.
func ValidCode(s string) bool { return codePattern.MatchString(s) }

// Errors returned by the meeting module.
var (
	ErrCodeTaken = errors.New("meeting code collision")

	errInvalidTitle    = apperr.New(apperr.KindInvalid, "invalid_title", "Title must be at most 120 characters.")
	errInvalidPassword = apperr.New(apperr.KindInvalid, "invalid_password", "Password must be 8–64 characters.")
	errInvalidExpiry   = apperr.New(apperr.KindInvalid, "invalid_expiry", "Meeting duration is out of range.")
	errInvalidName     = apperr.New(apperr.KindInvalid, "invalid_display_name", "Name must be 1–40 characters.")
)
