// Package resilience wraps remote calls with circuit breakers, per-call
// timeouts and bounded retries (PLAN.md §10). Use it only for network
// dependencies outside our control (LiveKit), never for DB/Redis.
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/sony/gobreaker/v2"
)

// Settings configure a breaker.
type Settings struct {
	Name             string
	FailureThreshold uint32
	OpenTimeout      time.Duration
	CallTimeout      time.Duration
	// IsSuccessful classifies errors that must not trip the breaker
	// (client errors, not-found, caller cancellation).
	IsSuccessful func(error) bool
	// OnState receives 0 closed, 1 half-open, 2 open.
	OnState func(name string, state float64)
}

// Breaker is a named circuit breaker with a per-call timeout.
type Breaker struct {
	cb      *gobreaker.CircuitBreaker[any]
	timeout time.Duration
}

// NewBreaker creates a breaker that trips on consecutive failures or a high
// failure ratio, and probes recovery with a few half-open requests.
func NewBreaker(s Settings) *Breaker {
	cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:         s.Name,
		MaxRequests:  3,
		Interval:     30 * time.Second,
		Timeout:      s.OpenTimeout,
		ReadyToTrip:  tripper(s.FailureThreshold),
		IsSuccessful: s.IsSuccessful,
		OnStateChange: func(name string, _, to gobreaker.State) {
			if s.OnState != nil {
				s.OnState(name, stateValue(to))
			}
		},
	})
	return &Breaker{cb: cb, timeout: s.CallTimeout}
}

func tripper(threshold uint32) func(gobreaker.Counts) bool {
	return func(c gobreaker.Counts) bool {
		if c.ConsecutiveFailures >= threshold {
			return true
		}
		return c.Requests >= 20 && float64(c.TotalFailures)/float64(c.Requests) >= 0.5
	}
}

func stateValue(s gobreaker.State) float64 {
	switch s {
	case gobreaker.StateHalfOpen:
		return 1
	case gobreaker.StateOpen:
		return 2
	default:
		return 0
	}
}

// ErrOpen is returned when the breaker rejects a call without attempting it.
var ErrOpen = errors.New("circuit open")

// Do runs fn once through the breaker with the per-call timeout.
func (b *Breaker) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	_, err := b.cb.Execute(func() (any, error) {
		callCtx, cancel := context.WithTimeout(ctx, b.timeout)
		defer cancel()
		return nil, fn(callCtx)
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ErrOpen
	}
	return err
}

// Retry runs fn through the breaker up to 3 times with jittered exponential
// backoff. Only use it for idempotent operations. An open breaker and errors
// for which retryable returns false stop immediately (no retry storms).
func (b *Breaker) Retry(ctx context.Context, retryable func(error) bool, fn func(ctx context.Context) error) error {
	op := func() (struct{}, error) {
		err := b.Do(ctx, fn)
		if err != nil && (errors.Is(err, ErrOpen) || !retryable(err)) {
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, err
	}
	_, err := backoff.Retry(ctx, op, backoff.WithBackOff(newBackOff()), backoff.WithMaxTries(3))
	return err
}

func newBackOff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 100 * time.Millisecond
	b.MaxInterval = time.Second
	b.RandomizationFactor = 0.5
	b.Multiplier = 2
	return b
}
