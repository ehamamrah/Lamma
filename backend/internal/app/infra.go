package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/metrics"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
	"github.com/ehamamrah/lamma/backend/internal/platform/redisx"
	"github.com/ehamamrah/lamma/backend/internal/platform/telemetry"
)

// infra holds process-wide infrastructure clients, closed in reverse order.
type infra struct {
	metrics *metrics.Metrics
	pool    *pgxpool.Pool
	redis   *redis.Client
	tracing telemetry.Shutdown
}

func openInfra(ctx context.Context, cfg config.Config) (*infra, error) {
	in := &infra{metrics: metrics.New()}
	tracing, err := telemetry.Setup(ctx, cfg.OTLPEndpoint, Version)
	if err != nil {
		return nil, err
	}
	in.tracing = tracing
	if in.pool, err = postgres.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns); err != nil {
		return nil, errors.Join(err, in.Close()) //nolint:contextcheck // shutdown runs after the request context is gone
	}
	in.metrics.Registry.MustRegister(postgres.NewPoolCollector(in.pool))
	in.redis, err = redisx.Connect(cfg.RedisURL, in.metrics.RedisDuration, in.metrics.RedisErrors)
	if err != nil {
		return nil, errors.Join(err, in.Close()) //nolint:contextcheck // shutdown runs after the request context is gone
	}
	return in, nil
}

// Close flushes tracing, then closes Redis and Postgres.
func (in *infra) Close() error {
	var errs []error
	if in.tracing != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		errs = append(errs, in.tracing(ctx))
		cancel()
	}
	if in.redis != nil {
		errs = append(errs, in.redis.Close())
	}
	if in.pool != nil {
		in.pool.Close()
	}
	err := errors.Join(errs...)
	if err != nil {
		slog.Error("infra close", slog.Any("error", err))
		return fmt.Errorf("close infra: %w", err)
	}
	return nil
}
