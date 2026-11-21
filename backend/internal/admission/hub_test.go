package admission

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/ehamamrah/lamma/backend/internal/access"
)

func newHub(t *testing.T, limits Limits) *Hub {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewHub(rdb, limits, prometheus.NewGauge(prometheus.GaugeOpts{Name: "g"}))
}

func TestRegisterEnforcesCaps(t *testing.T) {
	h := newHub(t, Limits{PerIP: 2, Total: 3})
	var releases []func()
	for range 2 {
		_, rel, err := h.Register(uuid.New(), "ip-a")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, rel)
	}
	if _, _, err := h.Register(uuid.New(), "ip-a"); err == nil {
		t.Fatal("third stream from one IP must be rejected")
	}
	_, relB, err := h.Register(uuid.New(), "ip-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Register(uuid.New(), "ip-c"); err == nil {
		t.Fatal("total cap must be enforced")
	}
	releases[0]()
	if _, _, err := h.Register(uuid.New(), "ip-c"); err != nil {
		t.Fatalf("released slot must be reusable: %v", err)
	}
	relB()
}

func TestDecisionDeliveredAcrossPubSub(t *testing.T) {
	h := newHub(t, Limits{PerIP: 10, Total: 10})
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { _ = h.Run(ctx) })
	defer func() { cancel(); wg.Wait() }()

	id := uuid.New()
	ch, release, _ := h.Register(id, "ip")
	defer release()
	deadline := time.After(3 * time.Second)
	for {
		h.Decided(context.Background(), id, access.StatusAdmitted)
		select {
		case got := <-ch:
			if got != access.StatusAdmitted {
				t.Fatalf("got %q", got)
			}
			return
		case <-time.After(50 * time.Millisecond): // subscription may not be active yet
		case <-deadline:
			t.Fatal("decision not delivered")
		}
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	h := newHub(t, Limits{PerIP: 1, Total: 1})
	h.Shutdown()
	h.Shutdown()
	select {
	case <-h.Done():
	default:
		t.Fatal("Done must be closed after Shutdown")
	}
}
