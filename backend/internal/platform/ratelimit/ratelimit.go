// Package ratelimit implements distributed GCRA rate limiting on Redis with a
// per-policy failure mode (PLAN.md §7, §10).
package ratelimit

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/clientip"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// fallbackFactor scales a per-client policy into a coarse per-replica budget
// used only while Redis is unreachable.
const fallbackFactor = 20

// Policy is a named rate with a failure mode for Redis outages.
type Policy struct {
	Name       string
	Rate       config.Rate
	FailClosed bool // abuse-sensitive: reject when Redis is down
}

// Limiter enforces policies; safe for concurrent use.
type Limiter struct {
	gcra       *redis_rate.Limiter
	rejections *prometheus.CounterVec
	mu         sync.Mutex
	fallback   map[string]*rate.Limiter
}

// New creates a Limiter backed by rdb.
func New(rdb *redis.Client, rejections *prometheus.CounterVec) *Limiter {
	return &Limiter{
		gcra:       redis_rate.NewLimiter(rdb),
		rejections: rejections,
		fallback:   make(map[string]*rate.Limiter),
	}
}

var errUnavailable = apperr.New(apperr.KindUnavailable, "rate_limit_unavailable",
	"Temporarily unavailable. Try again shortly.").WithRetry(5 * time.Second)

// Allow consumes one unit of p for key. It returns an *apperr.Error on rejection.
func (l *Limiter) Allow(ctx context.Context, p Policy, key string) error {
	limit := redis_rate.Limit{Rate: p.Rate.Count, Burst: p.Rate.Count, Period: p.Rate.Period}
	res, err := l.gcra.Allow(ctx, "rl:"+p.Name+":"+key, limit)
	if err != nil {
		return l.degraded(ctx, p, err)
	}
	if res.Allowed > 0 {
		return nil
	}
	return l.reject(p, res.RetryAfter)
}

func (l *Limiter) reject(p Policy, retryAfter time.Duration) error {
	l.rejections.WithLabelValues(p.Name).Inc()
	return apperr.ErrRateLimited.WithRetry(max(retryAfter, time.Second))
}

func (l *Limiter) degraded(ctx context.Context, p Policy, err error) error {
	slog.WarnContext(ctx, "rate limiter degraded", slog.String("policy", p.Name), slog.Any("error", err))
	if p.FailClosed {
		return errUnavailable
	}
	if l.fallbackFor(p).Allow() {
		return nil
	}
	return l.reject(p, time.Second)
}

func (l *Limiter) fallbackFor(p Policy) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := l.fallback[p.Name]; ok {
		return lim
	}
	perSec := float64(p.Rate.Count*fallbackFactor) / p.Rate.Period.Seconds()
	lim := rate.NewLimiter(rate.Limit(perSec), p.Rate.Count*fallbackFactor)
	l.fallback[p.Name] = lim
	return lim
}

// KeyFunc derives the limit key from a request; "" skips limiting.
type KeyFunc func(r *http.Request) string

// ByIP keys on the client IP (IPv6 /64).
func ByIP(r *http.Request) string { return clientip.Key(clientip.FromContext(r.Context())) }

// Middleware enforces p on every request using key.
func (l *Limiter) Middleware(p Policy, key KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := key(r)
			if k == "" {
				next.ServeHTTP(w, r)
				return
			}
			if err := l.Allow(r.Context(), p, k); err != nil {
				httpx.Error(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
