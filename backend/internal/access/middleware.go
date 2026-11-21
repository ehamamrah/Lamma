package access

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
	"github.com/ehamamrah/lamma/backend/internal/platform/logging"
)

type ctxKey struct{}

// SessionResolver is the read model the middleware depends on.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (Principal, error)
}

// LoadPrincipal resolves the session cookie, if present, for routes under
// /meetings/{code}. The principal must belong to the meeting in the URL.
// Requests without a valid session continue anonymously.
func LoadPrincipal(res SessionResolver, cookies Cookies) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := resolve(r, res, cookies)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			ctx := logging.With(context.WithValue(r.Context(), ctxKey{}, p),
				slog.String("meeting_id", p.MeetingID.String()),
				slog.String("participant_id", p.ParticipantID.String()))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolve(r *http.Request, res SessionResolver, cookies Cookies) (Principal, bool) {
	token, ok := cookies.Read(r)
	if !ok {
		return Principal{}, false
	}
	p, err := res.Resolve(r.Context(), token)
	if err != nil {
		if !errors.Is(err, ErrUnknownSession) {
			slog.ErrorContext(r.Context(), "session resolve failed", slog.Any("error", err))
		}
		return Principal{}, false
	}
	return p, p.Code == chi.URLParam(r, "code")
}

// FromContext returns the request principal, if any.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// Require rejects requests whose principal may not perform a.
func Require(a Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if !ok {
				httpx.Error(w, r, apperr.ErrNoSession)
				return
			}
			if !Can(p, a, time.Now()) {
				httpx.Error(w, r, deniedError(p))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func deniedError(p Principal) error {
	switch {
	case !p.MeetingOpen(time.Now()):
		return apperr.ErrNotFound
	case p.Status == StatusRemoved:
		return apperr.New(apperr.KindForbidden, "removed", "You were removed from this meeting.")
	case p.Status == StatusDenied:
		return apperr.New(apperr.KindForbidden, "denied", "The host declined your request to join.")
	default:
		return apperr.ErrForbidden
	}
}
