// Package apperr defines transport-agnostic application errors. Domain and
// service code return these; the HTTP layer maps Kind to a status code.
package apperr

import (
	"errors"
	"time"
)

// Kind classifies an error for the transport layer.
type Kind int

// Error kinds, mapped to HTTP statuses in httpx.
const (
	KindInternal Kind = iota
	KindInvalid
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindRateLimited
	KindUnavailable
)

// Error is a safe-to-expose error with a stable machine-readable code.
type Error struct {
	Kind       Kind
	Code       string
	Message    string
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// New creates an application error.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// WithRetry returns a copy carrying a Retry-After hint.
func (e *Error) WithRetry(d time.Duration) *Error {
	c := *e
	c.RetryAfter = d
	return &c
}

// As extracts an *Error from err.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// IsKind reports whether err is an application error of kind k.
func IsKind(err error, k Kind) bool {
	e, ok := As(err)
	return ok && e.Kind == k
}

// Shared errors used by several modules.
var (
	ErrNotFound    = New(KindNotFound, "not_found", "The meeting does not exist or has ended.")
	ErrNoSession   = New(KindUnauthorized, "session_required", "Join the meeting first.")
	ErrForbidden   = New(KindForbidden, "forbidden", "You are not allowed to do that.")
	ErrBusy        = New(KindUnavailable, "server_busy", "The server is busy. Try again shortly.")
	ErrRateLimited = New(KindRateLimited, "rate_limited", "Too many requests. Slow down.")
	ErrLiveKitDown = New(KindUnavailable, "livekit_unavailable", "Media service is temporarily unavailable.")
)
