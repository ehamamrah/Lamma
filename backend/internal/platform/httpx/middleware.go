package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/logging"
	"github.com/ehamamrah/lamma/backend/internal/platform/metrics"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// RequestID accepts a well-formed X-Request-ID (set by Nginx) or generates one,
// echoes it, and adds it to the log context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			id = newID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := logging.With(r.Context(), slog.String("request_id", id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Recover converts panics into 500 responses and logs the stack.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", slog.Any("panic", v), slog.String("stack", string(debug.Stack())))
				Error(w, r, apperr.New(apperr.KindInternal, "internal", "Something went wrong."))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Observe logs each request and records RED metrics keyed by the chi route
// pattern (never the raw path: meeting codes must not become labels or logs).
func Observe(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			route := routePattern(r)
			m.HTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(ww.Status())).Inc()
			m.HTTPDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
			logRequest(r, route, ww.Status(), time.Since(start))
		})
	}
}

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
		return rc.RoutePattern()
	}
	return "unmatched"
}

func logRequest(r *http.Request, route string, status int, d time.Duration) {
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	}
	slog.Log(r.Context(), level, "http request",
		slog.String("method", r.Method), slog.String("route", route),
		slog.Int("status", status), slog.Duration("duration", d))
}

// SecurityHeaders sets defensive headers on every API response.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

var errCrossSite = apperr.New(apperr.KindForbidden, "cross_site_request", "Cross-site requests are not allowed.")

// SameOrigin rejects unsafe cross-site requests (CSRF defence in depth on top
// of SameSite=Strict cookies). Allowed: same-origin fetches, or an Origin in
// the explicit allowlist.
func SameOrigin(publicOrigin string, allowed []string) func(http.Handler) http.Handler {
	origins := append([]string{publicOrigin}, allowed...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) || originAllowed(r, origins) {
				next.ServeHTTP(w, r)
				return
			}
			Error(w, r, errCrossSite)
		})
	}
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func originAllowed(r *http.Request, origins []string) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return slices.Contains(origins, origin)
	}
	site := r.Header.Get("Sec-Fetch-Site")
	return site == "same-origin" || site == "none"
}

// CORS answers preflights and sets headers for explicitly allowed origins only.
func CORS(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowed, origin) {
				next.ServeHTTP(w, r)
				return
			}
			writeCORSHeaders(w.Header(), origin)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeCORSHeaders(h http.Header, origin string) {
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
	h.Set("Access-Control-Max-Age", "600")
	h.Add("Vary", "Origin")
}
