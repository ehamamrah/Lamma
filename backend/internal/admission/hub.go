// Package admission delivers waiting-room decisions to waiting browsers over
// SSE. One Redis PSUBSCRIBE per replica feeds an in-process hub, so streams
// never hold their own Redis connection (PLAN.md §7, ADR-015).
package admission

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/platform/apperr"
)

const channelPrefix = "admission:"

// Limits bound concurrent streams (PLAN.md §10).
type Limits struct {
	PerIP int
	Total int
}

// Hub fans out decisions published on Redis to registered listeners.
type Hub struct {
	rdb      *redis.Client
	limits   Limits
	gauge    prometheus.Gauge
	mu       sync.Mutex
	subs     map[uuid.UUID]map[chan access.Status]struct{}
	perIP    map[string]int
	total    int
	done     chan struct{}
	shutOnce sync.Once
}

// NewHub creates a Hub.
func NewHub(rdb *redis.Client, limits Limits, gauge prometheus.Gauge) *Hub {
	return &Hub{
		rdb: rdb, limits: limits, gauge: gauge,
		subs:  make(map[uuid.UUID]map[chan access.Status]struct{}),
		perIP: make(map[string]int),
		done:  make(chan struct{}),
	}
}

var errTooManyStreams = apperr.New(apperr.KindRateLimited, "too_many_streams",
	"Too many open waiting-room connections.").WithRetry(10 * time.Second)

// Register subscribes to decisions for participant id. release must be called.
func (h *Hub) Register(id uuid.UUID, ipKey string) (<-chan access.Status, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= h.limits.Total || h.perIP[ipKey] >= h.limits.PerIP {
		return nil, nil, errTooManyStreams
	}
	ch := make(chan access.Status, 1)
	if h.subs[id] == nil {
		h.subs[id] = make(map[chan access.Status]struct{})
	}
	h.subs[id][ch] = struct{}{}
	h.perIP[ipKey]++
	h.total++
	h.gauge.Set(float64(h.total))
	return ch, func() { h.release(id, ipKey, ch) }, nil
}

func (h *Hub) release(id uuid.UUID, ipKey string, ch chan access.Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[id], ch)
	if len(h.subs[id]) == 0 {
		delete(h.subs, id)
	}
	if h.perIP[ipKey]--; h.perIP[ipKey] <= 0 {
		delete(h.perIP, ipKey)
	}
	h.total--
	h.gauge.Set(float64(h.total))
}

// Done is closed when the hub shuts down; streams must then end.
func (h *Hub) Done() <-chan struct{} { return h.done }

// Shutdown ends all streams (wired to http.Server.RegisterOnShutdown).
func (h *Hub) Shutdown() { h.shutOnce.Do(func() { close(h.done) }) }

// Decided publishes a decision for a waiting participant (after commit).
func (h *Hub) Decided(ctx context.Context, id uuid.UUID, status access.Status) {
	if err := h.rdb.Publish(ctx, channelPrefix+id.String(), string(status)).Err(); err != nil {
		slog.WarnContext(ctx, "publish admission decision", slog.Any("error", err))
	}
}

// Run consumes the pattern subscription until ctx is cancelled. go-redis
// reconnects the subscription automatically.
func (h *Hub) Run(ctx context.Context) error {
	ps := h.rdb.PSubscribe(ctx, channelPrefix+"*")
	defer func() { _ = ps.Close() }()
	msgs := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case m, ok := <-msgs:
			if !ok {
				return nil
			}
			h.dispatch(m.Channel, access.Status(m.Payload))
		}
	}
}

func (h *Hub) dispatch(channel string, status access.Status) {
	id, err := uuid.Parse(strings.TrimPrefix(channel, channelPrefix))
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[id] {
		select {
		case ch <- status:
		default: // a decision is already pending for this listener
		}
	}
}
