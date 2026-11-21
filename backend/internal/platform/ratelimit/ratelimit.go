// Package ratelimit implements distributed GCRA rate limiting on Redis with a
// per-policy failure mode (PLAN.md §7, §10).
package ratelimit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
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

const (
	// redisBudget bounds the time a request waits on Redis for a decision.
	redisBudget = 150 * time.Millisecond
	// skipWindow stops consulting Redis briefly after an error, so a
	// blackholed Redis does not add latency to every request.
	skipWindow = time.Second
	// maxFallbackKeys bounds the per-replica fallback limiter's memory.
	maxFallbackKeys = 10000
)

// Policy is a named rate with a failure mode for Redis outages.
type Policy struct {
	Name       string
	Rate       config.Rate
	FailClosed bool // abuse-sensitive: reject when Redis is down
}

// Limiter enforces policies; safe for concurrent use.
type Limiter struct {
	gcra       *redis_rate.Limiter
	secret     []byte
	rejections *prometheus.CounterVec
	skipUntil  atomic.Int64
	mu         sync.Mutex
	fallback   map[string]*rate.Limiter
}

// New creates a Limiter backed by rdb. Keys are HMAC'd with secret so no raw
// client addresses are stored in Redis.
func New(rdb *redis.Client, secret []byte, rejections *prometheus.CounterVec) *Limiter {
	return &Limiter{
		gcra:       redis_rate.NewLimiter(rdb),
		secret:     secret,
		rejections: rejections,
		fallback:   make(map[string]*rate.Limiter),
	}
}

var errUnavailable = apperr.New(apperr.KindUnavailable, "rate_limit_unavailable",
	"Temporarily unavailable. Try again shortly.").WithRetry(5 * time.Second)

// Allow consumes one unit of p for key. It returns an *apperr.Error on rejection.
func (l *Limiter) Allow(ctx context.Context, p Policy, key string) error {
	id := l.hashKey(p.Name, key)
	if time.Now().UnixNano() < l.skipUntil.Load() {
		return l.degraded(ctx, p, id, nil)
	}
	res, err := l.allowRedis(ctx, p, id)
	if err != nil {
		l.skipUntil.Store(time.Now().Add(skipWindow).UnixNano())
		return l.degraded(ctx, p, id, err)
	}
	if res.Allowed > 0 {
		return nil
	}
	return l.reject(p, res.RetryAfter)
}

func (l *Limiter) allowRedis(ctx context.Context, p Policy, id string) (*redis_rate.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, redisBudget)
	defer cancel()
	limit := redis_rate.Limit{Rate: p.Rate.Count, Burst: p.Rate.Count, Period: p.Rate.Period}
	return l.gcra.Allow(ctx, "rl:"+p.Name+":"+id, limit)
}

func (l *Limiter) hashKey(policy, key string) string {
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(policy + "\x00" + key))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

func (l *Limiter) reject(p Policy, retryAfter time.Duration) error {
	l.rejections.WithLabelValues(p.Name).Inc()
	return apperr.ErrRateLimited.WithRetry(max(retryAfter, time.Second))
}

// degraded applies the policy's failure mode while Redis is unavailable:
// fail-closed policies reject; fail-open ones use a per-client in-memory limiter.
func (l *Limiter) degraded(ctx context.Context, p Policy, id string, err error) error {
	if err != nil {
		slog.WarnContext(ctx, "rate limiter degraded", slog.String("policy", p.Name), slog.Any("error", err))
	}
	if p.FailClosed {
		return errUnavailable
	}
	if l.fallbackFor(p, id).Allow() {
		return nil
	}
	return l.reject(p, p.Rate.Period/time.Duration(p.Rate.Count))
}

func (l *Limiter) fallbackFor(p Policy, id string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := l.fallback[id]; ok {
		return lim
	}
	if len(l.fallback) >= maxFallbackKeys {
		clear(l.fallback) // bounded memory; brief leniency is acceptable in degraded mode
	}
	lim := rate.NewLimiter(rate.Every(p.Rate.Period/time.Duration(p.Rate.Count)), p.Rate.Count)
	l.fallback[id] = lim
	return lim
}

// Check is the port services depend on: consume one unit for key or return
// an *apperr.Error. Services never import this package directly.
type Check func(ctx context.Context, key string) error

// For binds a policy, for injection into services.
func (l *Limiter) For(p Policy) Check {
	return func(ctx context.Context, key string) error { return l.Allow(ctx, p, key) }
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
