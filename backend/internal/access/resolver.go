package access

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ehamamrah/lamma/backend/internal/access/store"
	"github.com/ehamamrah/lamma/backend/internal/platform/postgres"
)

// ErrUnknownSession means the token matches no participant.
var ErrUnknownSession = errors.New("unknown session")

// Resolver maps session tokens to principals with a single query.
type Resolver struct {
	tx *postgres.TxManager
}

// NewResolver creates a Resolver.
func NewResolver(tx *postgres.TxManager) *Resolver { return &Resolver{tx: tx} }

// Resolve returns the principal for a raw session token.
func (r *Resolver) Resolve(ctx context.Context, token string) (Principal, error) {
	row, err := store.New(r.tx.Conn(ctx)).ResolveSession(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnknownSession
	}
	if err != nil {
		return Principal{}, fmt.Errorf("resolve session: %w", err)
	}
	return toPrincipal(row), nil
}

func toPrincipal(row store.ResolveSessionRow) Principal {
	return Principal{
		ParticipantID: row.ParticipantID,
		MeetingID:     row.MeetingID,
		Code:          row.Code,
		DisplayName:   row.DisplayName,
		Role:          Role(row.Role),
		Status:        Status(row.Status),
		MeetingActive: row.MeetingStatus == store.MeetingStatusActive,
		ExpiresAt:     row.ExpiresAt,
		Locked:        row.Locked,
		WaitingRoom:   row.WaitingRoom,
	}
}
