package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is satisfied by both the pool and a transaction; sqlc stores use it.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type txKey struct{}

// TxManager runs functions in a transaction carried by the context, so that
// services stay free of pgx. Rules: stores obtain their connection only via
// Conn(ctx); a transaction may span modules only through service calls (never
// another module's store); fn must not perform network calls (LiveKit, Redis)
// and must not share ctx with other goroutines.
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager creates a TxManager.
func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// InTx runs fn inside a transaction; nested calls join the outer transaction.
func (m *TxManager) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { //nolint:contextcheck // rollback uses a fresh context on purpose
		if p := recover(); p != nil {
			_ = rollback(tx)
			panic(p)
		}
	}()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return errors.Join(err, rollback(tx)) //nolint:contextcheck // rollback uses a fresh context on purpose
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Conn returns the context transaction if present, else the pool.
func (m *TxManager) Conn(ctx context.Context) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return m.pool
}

func rollback(tx pgx.Tx) error {
	// A fresh context: the request context may already be cancelled.
	err := tx.Rollback(context.Background())
	if err == nil || errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return fmt.Errorf("rollback: %w", err)
}

// IsUniqueViolation reports whether err is a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
