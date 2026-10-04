package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
	// ON DELETE RESTRICT reports restrict_violation (verified on Postgres 18), not foreign_key_violation.
	sqlStateRestrictViolation = "23001"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func IsUniqueViolation(err error) bool { return hasSQLState(err, sqlStateUniqueViolation) }
func IsForeignKeyViolation(err error) bool {
	return hasSQLState(err, sqlStateForeignKeyViolation) || hasSQLState(err, sqlStateRestrictViolation)
}

func hasSQLState(err error, state string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == state
}
