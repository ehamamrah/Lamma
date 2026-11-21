package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	drainDelay      = 5 * time.Second // let load balancers observe readiness=503
	shutdownTimeout = 20 * time.Second
)

// Worker is a background loop that must return when ctx is cancelled.
type Worker func(ctx context.Context) error

// server owns the HTTP listeners and background workers.
type server struct {
	api        *http.Server
	metrics    *http.Server
	workers    []Worker
	drain      func()
	drainDelay time.Duration
}

// Run serves until ctx is cancelled (SIGTERM/SIGINT) or a component fails,
// then shuts down in order: readiness → drain → HTTP → workers.
func (s *server) Run(ctx context.Context) error {
	workerCtx, cancelWorkers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWorkers()
	g, gctx := errgroup.WithContext(workerCtx)
	for _, w := range s.workers {
		g.Go(func() error { return w(gctx) })
	}
	g.Go(func() error { return listen(s.api) })
	g.Go(func() error { return listen(s.metrics) })
	select {
	case <-ctx.Done():
	case <-gctx.Done():
	}
	err := s.shutdown()
	cancelWorkers()
	return errors.Join(err, g.Wait())
}

func (s *server) shutdown() error {
	slog.Info("shutting down")
	s.drain()
	time.Sleep(s.drainDelay)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return errors.Join(s.api.Shutdown(ctx), s.metrics.Shutdown(ctx))
}

func listen(srv *http.Server) error {
	slog.Info("listening", slog.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}
