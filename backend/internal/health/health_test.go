package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func probe(err error) func(context.Context) error {
	return func(context.Context) error { return err }
}

func TestReady(t *testing.T) {
	down := errors.New("down")
	tests := []struct {
		name   string
		checks []Check
		drain  bool
		want   int
	}{
		{"all up", []Check{{Name: "pg", Required: true, Probe: probe(nil)}}, false, 200},
		{"required down", []Check{{Name: "pg", Required: true, Probe: probe(down)}}, false, 503},
		{"optional down stays ready", []Check{
			{Name: "pg", Required: true, Probe: probe(nil)},
			{Name: "redis", Required: false, Probe: probe(down)},
		}, false, 200},
		{"draining", []Check{{Name: "pg", Required: true, Probe: probe(nil)}}, true, 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.checks...)
			if tt.drain {
				s.Drain()
			}
			rec := httptest.NewRecorder()
			s.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestLiveNeverTouchesDependencies(t *testing.T) {
	s := New(Check{Name: "pg", Required: true, Probe: func(context.Context) error {
		t.Fatal("liveness must not probe dependencies")
		return nil
	}})
	rec := httptest.NewRecorder()
	s.Live(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
