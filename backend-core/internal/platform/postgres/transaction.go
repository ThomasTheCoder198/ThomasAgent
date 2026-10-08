package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// TxStarter is a pool (or anything) that can open a transaction.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTransaction runs fn in one transaction: it commits when fn succeeds and rolls back otherwise.
// Services own transactions; repositories receive the tx as their DBTX.
func WithTransaction(ctx context.Context, db TxStarter, fn func(tx pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return errors.Normalize(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}
