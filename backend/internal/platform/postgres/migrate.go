package postgres

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies (direction "up") or rolls back one step ("down") using the
// embedded migration files. It runs in the one-shot migrate container only.
func Migrate(files fs.FS, databaseURL, direction string) error {
	src, err := iofs.New(files, "migrations")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	err = runDirection(m, direction)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

func runDirection(m *migrate.Migrate, direction string) error {
	switch direction {
	case "up":
		return m.Up()
	case "down":
		return m.Steps(-1)
	default:
		return fmt.Errorf("unknown migrate direction %q (want up|down)", direction)
	}
}

func toPgx5URL(url string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(url, prefix) {
			return "pgx5://" + strings.TrimPrefix(url, prefix)
		}
	}
	return url
}
