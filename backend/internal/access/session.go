package access

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

const tokenBytes = 32

// NewSessionToken returns a 256-bit random token and its SHA-256 hash.
// Only the hash is stored; the raw token lives in the HttpOnly cookie.
func NewSessionToken() (token string, hash []byte, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken hashes a raw session token for lookup. SHA-256 (not a slow KDF)
// is correct for 256-bit random tokens.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Cookies writes and reads the per-meeting session cookie (PLAN.md §5).
type Cookies struct {
	Secure bool
}

// Name is "__Secure-lamma_sid" when Secure, else "lamma_sid" (dev over http).
func (c Cookies) Name() string {
	if c.Secure {
		return "__Secure-lamma_sid"
	}
	return "lamma_sid"
}

// Set scopes the cookie to the meeting's API path so a browser can hold
// independent sessions in many meetings.
func (c Cookies) Set(w http.ResponseWriter, code, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is false only in dev over http; production guard enforces true
		Name:     c.Name(),
		Value:    token,
		Path:     "/api/v1/meetings/" + code,
		Expires:  expires,
		MaxAge:   max(1, int(time.Until(expires).Seconds())),
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// Read returns the raw session token from r, if any.
func (c Cookies) Read(r *http.Request) (string, bool) {
	ck, err := r.Cookie(c.Name())
	if err != nil || len(ck.Value) == 0 || len(ck.Value) > 64 {
		return "", false
	}
	return ck.Value, true
}
