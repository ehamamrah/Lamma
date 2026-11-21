// Package redisx creates the shared Redis client with timeouts and a
// latency/error metrics hook.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// Connect parses url, applies timeouts and installs the metrics hook.
// Connectivity is not required at startup: Redis failures degrade gracefully.
func Connect(url string, latency *prometheus.HistogramVec, errs prometheus.Counter) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = 500 * time.Millisecond
	opts.WriteTimeout = 500 * time.Millisecond
	opts.PoolSize = 50
	opts.MinIdleConns = 2
	client := redis.NewClient(opts)
	client.AddHook(metricsHook{latency: latency, errs: errs})
	return client, nil
}

type metricsHook struct {
	latency *prometheus.HistogramVec
	errs    prometheus.Counter
}

func (h metricsHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h metricsHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		h.observe(cmd.Name(), start, err)
		return err
	}
}

func (h metricsHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		h.observe("pipeline", start, err)
		return err
	}
}

func (h metricsHook) observe(op string, start time.Time, err error) {
	h.latency.WithLabelValues(op).Observe(time.Since(start).Seconds())
	if err != nil && !errors.Is(err, redis.Nil) {
		h.errs.Inc()
	}
}
