package auth

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	ownerEmail    = "thomas@example.com"
	ownerPassword = "correct horse battery"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// swappableDB lets a test inject a failing database after the owner is bootstrapped.
type swappableDB struct{ inner transactionDB }

func (db *swappableDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.inner.Exec(ctx, sql, args...)
}
func (db *swappableDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.inner.Query(ctx, sql, args...)
}
func (db *swappableDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.inner.QueryRow(ctx, sql, args...)
}
func (db *swappableDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.inner.Begin(ctx)
}

type rollbackFailureDB struct {
	transactionDB
	failure error
}

func (db rollbackFailureDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.transactionDB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return rollbackFailureTx{Tx: tx, failure: db.failure}, nil
}

type rollbackFailureTx struct {
	pgx.Tx
	failure error
}

func (tx rollbackFailureTx) Rollback(ctx context.Context) error {
	return stderrors.Join(tx.Tx.Rollback(ctx), tx.failure)
}

type authFixture struct {
	*Service
	repository *Repository
	db         *swappableDB
}

func newTestService(t *testing.T) (*authFixture, *clock) {
	t.Helper()
	pool := pgtest.Start(t)
	testClock := &clock{t: time.Now()}
	authConfig := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	repository := NewRepository(pool)
	db := &swappableDB{inner: pool}
	fixture := &authFixture{Service: NewService(repository, db, authConfig, testClock.now), repository: repository, db: db}
	require.NoError(t, fixture.EnsureOwner(tenant.WithID(t.Context(), tenant.PlatformID)))
	return fixture, testClock
}
