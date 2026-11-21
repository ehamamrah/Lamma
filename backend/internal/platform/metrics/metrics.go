// Package metrics defines every Prometheus metric the service exports
// (PLAN.md §12). Metrics are registered on an explicit registry, not the global one.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics groups all application metrics.
type Metrics struct {
	Registry            *prometheus.Registry
	HTTPRequests        *prometheus.CounterVec
	HTTPDuration        *prometheus.HistogramVec
	RateLimitRejections *prometheus.CounterVec
	MeetingsActive      prometheus.Gauge
	ParticipantsInRoom  prometheus.Gauge
	LiveKitFailures     *prometheus.CounterVec
	BreakerState        *prometheus.GaugeVec
	RedisDuration       *prometheus.HistogramVec
	RedisErrors         prometheus.Counter
	SSEStreams          prometheus.Gauge
}

// New creates and registers all metrics plus Go/process collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{Registry: reg}
	m.registerHTTP()
	m.registerDomain()
	m.registerDependencies()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

// Handler serves the registry in Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

func (m *Metrics) registerHTTP() {
	m.HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total", Help: "HTTP requests by route, method and status.",
	}, []string{"route", "method", "status"})
	m.HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds", Help: "HTTP request latency.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"route", "method"})
	m.RateLimitRejections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ratelimit_rejections_total", Help: "Requests rejected by rate limiting.",
	}, []string{"policy"})
	m.Registry.MustRegister(m.HTTPRequests, m.HTTPDuration, m.RateLimitRejections)
}

func (m *Metrics) registerDomain() {
	m.MeetingsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "meetings_active", Help: "Active meetings (global; aggregate with max()).",
	})
	m.ParticipantsInRoom = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "participants_in_room", Help: "Participants connected to LiveKit (global; max()).",
	})
	m.SSEStreams = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sse_streams", Help: "Open waiting-room SSE streams on this replica.",
	})
	m.Registry.MustRegister(m.MeetingsActive, m.ParticipantsInRoom, m.SSEStreams)
}

func (m *Metrics) registerDependencies() {
	m.LiveKitFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "livekit_operation_failures_total", Help: "Failed LiveKit server API calls.",
	}, []string{"op"})
	m.BreakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state", Help: "0 closed, 1 half-open, 2 open.",
	}, []string{"name"})
	m.RedisDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "redis_operation_duration_seconds", Help: "Redis command latency.",
		Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .5},
	}, []string{"op"})
	m.RedisErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "redis_errors_total", Help: "Redis command errors (excluding nil replies).",
	})
	m.Registry.MustRegister(m.LiveKitFailures, m.BreakerState, m.RedisDuration, m.RedisErrors)
}
