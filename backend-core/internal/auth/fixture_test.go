package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	ownerEmail    = "thomas@example.com"
	ownerPassword = "correct horse battery"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type transactionDB interface {
	postgres.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

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

type authFixture struct {
	*service.Service
	repo *repository.Repository
	db   *swappableDB
}

func newService(t *testing.T) (*authFixture, *clock) {
	t.Helper()
	pool := pgtest.Start(t)
	c := &clock{t: time.Now()}
	cfg := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	repo := repository.NewAuthRepository(pool)
	db := &swappableDB{inner: pool}
	fixture := &authFixture{Service: service.NewAuthService(repo, db, cfg, c.now), repo: repo, db: db}
	require.NoError(t, fixture.EnsureOwner(tenant.WithID(t.Context(), tenant.PlatformID)))
	return fixture, c
}
