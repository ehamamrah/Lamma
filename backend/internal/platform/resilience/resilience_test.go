package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

var errRemote = errors.New("remote failure")
var errClient = errors.New("not found")

func newTestBreaker(states *[]float64) *Breaker {
	return NewBreaker(Settings{
		Name: "test", FailureThreshold: 3, OpenTimeout: 50 * time.Millisecond,
		CallTimeout:  100 * time.Millisecond,
		IsSuccessful: func(err error) bool { return err == nil || errors.Is(err, errClient) },
		OnState:      func(_ string, s float64) { *states = append(*states, s) },
	})
}

func fail(err error) func(context.Context) error {
	return func(context.Context) error { return err }
}

func TestBreakerLifecycle(t *testing.T) {
	var states []float64
	b := newTestBreaker(&states)
	ctx := context.Background()
	for range 3 {
		_ = b.Do(ctx, fail(errRemote))
	}
	if err := b.Do(ctx, fail(nil)); !errors.Is(err, ErrOpen) {
		t.Fatalf("after 3 failures want ErrOpen, got %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := b.Do(ctx, fail(nil)); err != nil {
		t.Fatalf("half-open probe should run and succeed: %v", err)
	}
	want := []float64{2, 1}
	if len(states) < 2 || states[0] != want[0] || states[1] != want[1] {
		t.Fatalf("state transitions = %v, want prefix %v", states, want)
	}
}

func TestClientErrorsDoNotTrip(t *testing.T) {
	var states []float64
	b := newTestBreaker(&states)
	for range 10 {
		if err := b.Do(context.Background(), fail(errClient)); !errors.Is(err, errClient) {
			t.Fatalf("client error must pass through: %v", err)
		}
	}
	if len(states) != 0 {
		t.Fatalf("breaker must stay closed, transitions: %v", states)
	}
}

func TestDoAppliesCallTimeout(t *testing.T) {
	var states []float64
	b := newTestBreaker(&states)
	err := b.Do(context.Background(), func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name      string
		failures  int32
		retryable bool
		wantCalls int32
		wantErr   bool
	}{
		{"succeeds after transient failures", 2, true, 3, false},
		{"bounded to 3 attempts", 10, true, 3, true},
		{"non-retryable stops immediately", 10, false, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var states []float64
			b := NewBreaker(Settings{Name: "r", FailureThreshold: 100, OpenTimeout: time.Second, CallTimeout: time.Second,
				OnState: func(string, float64) { states = append(states, 0) }})
			var calls atomic.Int32
			err := b.Retry(context.Background(), func(error) bool { return tt.retryable }, func(context.Context) error {
				if calls.Add(1) <= tt.failures {
					return errRemote
				}
				return nil
			})
			if calls.Load() != tt.wantCalls || (err != nil) != tt.wantErr {
				t.Fatalf("calls=%d err=%v; want calls=%d err=%v", calls.Load(), err, tt.wantCalls, tt.wantErr)
			}
		})
	}
}

func TestRetryStopsOnOpenBreaker(t *testing.T) {
	var states []float64
	b := newTestBreaker(&states)
	for range 3 {
		_ = b.Do(context.Background(), fail(errRemote))
	}
	var calls atomic.Int32
	err := b.Retry(context.Background(), func(error) bool { return true }, func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if !errors.Is(err, ErrOpen) || calls.Load() != 0 {
		t.Fatalf("want immediate ErrOpen without calls, got err=%v calls=%d", err, calls.Load())
	}
}
