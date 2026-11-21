// Package health serves liveness and readiness probes (PLAN.md §12).
// Liveness never touches dependencies; readiness fails only on required ones.
package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
)

// Check probes one dependency. Required checks gate readiness; optional ones
// are reported only (they degrade gracefully).
type Check struct {
	Name     string
	Required bool
	Probe    func(ctx context.Context) error
}

// Service evaluates checks with a short cache to avoid probe storms.
type Service struct {
	checks   []Check
	draining atomic.Bool
	ttl      time.Duration
	mu       sync.Mutex
	cachedAt time.Time
	cached   report
}

type report struct {
	Ready        bool              `json:"ready"`
	Dependencies map[string]string `json:"dependencies"`
}

// New creates a health service.
func New(checks ...Check) *Service {
	return &Service{checks: checks, ttl: 2 * time.Second}
}

// Drain makes readiness fail so load balancers stop routing new traffic.
func (s *Service) Drain() { s.draining.Store(true) }

// Live always succeeds while the process can serve HTTP.
func (s *Service) Live(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready reports dependency state; 503 when draining or a required check fails.
func (s *Service) Ready(w http.ResponseWriter, r *http.Request) {
	rep := s.evaluate(r.Context())
	status := http.StatusOK
	if !rep.Ready || s.draining.Load() {
		status = http.StatusServiceUnavailable
	}
	httpx.JSON(w, status, rep)
}

func (s *Service) evaluate(ctx context.Context) report {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.cachedAt) < s.ttl {
		return s.cached
	}
	s.cached = s.runChecks(ctx)
	s.cachedAt = time.Now()
	return s.cached
}

func (s *Service) runChecks(ctx context.Context) report {
	rep := report{Ready: true, Dependencies: make(map[string]string, len(s.checks))}
	for _, c := range s.checks {
		// Detached: a client hanging up must not cache a false "down".
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
		err := c.Probe(probeCtx)
		cancel()
		rep.Dependencies[c.Name] = "up"
		if err != nil {
			rep.Dependencies[c.Name] = "down"
			rep.Ready = rep.Ready && !c.Required
		}
	}
	return rep
}
