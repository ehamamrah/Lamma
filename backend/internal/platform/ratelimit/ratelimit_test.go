package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
)

func newTestLimiter(t *testing.T) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	rej := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x"}, []string{"policy"})
	return New(rdb, []byte("test-secret"), rej), mr
}

func TestAllowEnforcesRatePerKey(t *testing.T) {
	l, _ := newTestLimiter(t)
	p := Policy{Name: "join", Rate: config.Rate{Count: 3, Period: time.Minute}}
	ctx := context.Background()
	for i := range 3 {
		if err := l.Allow(ctx, p, "1.2.3.4"); err != nil {
			t.Fatalf("request %d rejected: %v", i, err)
		}
	}
	err := l.Allow(ctx, p, "1.2.3.4")
	appErr, ok := apperr.As(err)
	if !ok || appErr.Kind != apperr.KindRateLimited || appErr.RetryAfter <= 0 {
		t.Fatalf("4th request: got %v, want rate_limited with Retry-After", err)
	}
	if err := l.Allow(ctx, p, "5.6.7.8"); err != nil {
		t.Fatalf("other key must have its own budget: %v", err)
	}
}

func TestAllowIsAtomicUnderConcurrency(t *testing.T) {
	l, _ := newTestLimiter(t)
	p := Policy{Name: "c", Rate: config.Rate{Count: 10, Period: time.Minute}}
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if l.Allow(context.Background(), p, "k") == nil {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if got := allowed.Load(); got != 10 {
		t.Fatalf("allowed %d of 50 concurrent requests, want exactly 10", got)
	}
}

func TestRedisOutageFailureModes(t *testing.T) {
	tests := []struct {
		name       string
		failClosed bool
		wantKind   apperr.Kind
		wantErr    bool
	}{
		{"fail-open degrades to per-client local limiter", false, 0, false},
		{"fail-closed rejects with 503", true, apperr.KindUnavailable, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, mr := newTestLimiter(t)
			mr.Close()
			p := Policy{Name: "p", Rate: config.Rate{Count: 1, Period: time.Minute}, FailClosed: tt.failClosed}
			err := l.Allow(context.Background(), p, "k")
			if (err != nil) != tt.wantErr || (tt.wantErr && !apperr.IsKind(err, tt.wantKind)) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestFallbackIsPerClient(t *testing.T) {
	l, mr := newTestLimiter(t)
	mr.Close()
	p := Policy{Name: "g", Rate: config.Rate{Count: 2, Period: time.Minute}}
	ctx := context.Background()
	for range 2 {
		_ = l.Allow(ctx, p, "abuser")
	}
	if err := l.Allow(ctx, p, "abuser"); !apperr.IsKind(err, apperr.KindRateLimited) {
		t.Fatalf("abuser should be limited in degraded mode, got %v", err)
	}
	if err := l.Allow(ctx, p, "victim"); err != nil {
		t.Fatalf("other clients must keep their own budget in degraded mode: %v", err)
	}
}

// A client hanging up must not push the limiter into degraded mode (final review SP6-1).
func TestCancelledCallerDoesNotDegradeLimiter(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := Policy{Name: "c", Rate: config.Rate{Count: 1, Period: time.Minute}, FailClosed: true}
	_ = l.Allow(ctx, p, "k")
	if l.skipUntil.Load() != 0 {
		t.Fatal("a cancelled request must not trigger the Redis skip window")
	}
	if err := l.Allow(context.Background(), p, "k"); err != nil {
		t.Fatalf("other clients must still use Redis normally: %v", err)
	}
}

// Two API replicas share one budget through Redis (rate limiting across instances).
func TestLimitIsSharedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	rej := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x"}, []string{"policy"})
	newReplica := func() *Limiter {
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = rdb.Close() })
		return New(rdb, []byte("shared-secret"), rej)
	}
	a, b := newReplica(), newReplica()
	p := Policy{Name: "join", Rate: config.Rate{Count: 10, Period: time.Minute}}
	ctx := context.Background()
	for i := range 10 {
		replica := a
		if i%2 == 1 {
			replica = b
		}
		if err := replica.Allow(ctx, p, "1.2.3.4"); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if a.Allow(ctx, p, "1.2.3.4") == nil || b.Allow(ctx, p, "1.2.3.4") == nil {
		t.Fatal("the 11th request must be rejected on every replica")
	}
}
