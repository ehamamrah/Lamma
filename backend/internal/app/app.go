// Package app wires configuration, infrastructure and modules together and
// owns the process lifecycle. It is the only place that knows every module.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ehamamrah/lamma/backend/db"
	"github.com/ehamamrah/lamma/backend/internal/platform/config"
	"github.com/ehamamrah/lamma/backend/internal/platform/logging"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Main dispatches the subcommand and returns the process exit code.
func Main(ctx context.Context, args []string) int {
	cmd, rest := "serve", args
	if len(args) > 0 {
		cmd, rest = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(ctx)
	case "migrate":
		err = migrateCmd(rest)
	case "healthcheck":
		err = healthcheck(ctx)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	return exitCode(err)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	slog.Error("fatal", slog.Any("error", err))
	return 1
}

func serve(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	slog.SetDefault(logging.New(os.Stdout, cfg.LogLevel))
	slog.Info("starting", slog.String("version", Version), slog.Any("config", cfg))
	infra, err := openInfra(ctx, cfg)
	if err != nil {
		return err
	}
	defer infra.Close()
	return newServer(cfg, infra).Run(ctx)
}

func migrateCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	slog.SetDefault(logging.New(os.Stdout, cfg.LogLevel))
	direction := "up"
	if len(args) > 0 {
		direction = args[0]
	}
	url := cmpOr(cfg.MigrateURL, cfg.DatabaseURL)
	if err := postgres.Migrate(db.Migrations, url, direction); err != nil {
		return fmt.Errorf("migrate %s: %w", direction, err)
	}
	slog.Info("migrations applied", slog.String("direction", direction))
	return nil
}

// healthcheck lets distroless containers probe liveness without curl.
func healthcheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8000/health/live", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("liveness status %d", resp.StatusCode)
	}
	return nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
