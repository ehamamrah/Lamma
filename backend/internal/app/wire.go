package app

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ehamamrah/lamma/backend/internal/access"
	"github.com/ehamamrah/lamma/backend/internal/health"
	"github.com/ehamamrah/lamma/backend/internal/platform/clientip"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/httpx"
	"github.com/ehamamrah/lamma/backend/internal/platform/ratelimit"
)

// deps carries the cross-cutting components modules are built from.
type deps struct {
	cfg     config.Config
	infra   *infra
	limiter *ratelimit.Limiter
	health  *health.Service
}

func newServer(cfg config.Config, in *infra) *server {
	d := deps{
		cfg:     cfg,
		infra:   in,
		limiter: ratelimit.New(in.redis, cfg.OriginHashKey, in.metrics.RateLimitRejections),
		health:  newHealth(in),
	}
	rts, workers, onShutdown := buildModules(d)
	api := newHTTPServer(cfg.HTTPAddr, d.router(rts))
	for _, fn := range onShutdown {
		api.RegisterOnShutdown(fn)
	}
	return &server{
		api:        api,
		metrics:    newHTTPServer(cfg.MetricsAddr, in.metrics.Handler()),
		workers:    workers,
		drain:      d.health.Drain,
		drainDelay: drainDelayFor(cfg),
	}
}

func newHealth(in *infra) *health.Service {
	return health.New(
		health.Check{Name: "postgres", Required: true, Probe: in.pool.Ping},
		health.Check{Name: "redis", Probe: func(ctx context.Context) error { return in.redis.Ping(ctx).Err() }},
	)
}

func drainDelayFor(cfg config.Config) time.Duration {
	if cfg.IsProduction() {
		return drainDelay
	}
	return 0
}

func (d deps) router(rts routes) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID, httpx.Recover, clientip.NewResolver(d.cfg.TrustedProxies).Middleware)
	r.Use(httpx.Observe(d.infra.metrics), httpx.SecurityHeaders(d.cfg.IsProduction()))
	r.Get("/health/live", d.health.Live)
	r.Get("/health/ready", d.health.Ready)
	r.Route("/api/v1", func(r chi.Router) { d.apiRoutes(r, rts) })
	r.Route("/internal", rts.internal)
	return otelhttp.NewHandler(r, "http", otelhttp.WithFilter(isTraced))
}

func (d deps) apiRoutes(r chi.Router, rts routes) {
	r.Use(httpx.CORS(d.cfg.CORSOrigins), httpx.SameOrigin(publicOrigin(d.cfg.PublicBaseURL), d.cfg.CORSOrigins))
	r.Use(d.limiter.Middleware(ratelimit.Policy{Name: "general", Rate: d.cfg.Limits.General}, ratelimit.ByIP))
	for _, register := range rts.root {
		register(r)
	}
	r.Route("/meetings/{code}", func(r chi.Router) {
		r.Use(access.LoadPrincipal(rts.resolver, access.Cookies{Secure: d.cfg.CookieSecure}))
		for _, register := range rts.scoped {
			register(r)
		}
	})
}

func isTraced(r *http.Request) bool {
	return r.URL.Path != "/health/live" && r.URL.Path != "/health/ready"
}

func publicOrigin(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
