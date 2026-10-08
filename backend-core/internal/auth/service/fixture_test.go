package service

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
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

// swappableDB lets a test inject a failing database after the owner is bootstrapped.
type swappableDB struct{ inner TransactionDB }

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

// failingExecDB fails the first statement with the given prefix, also inside transactions.
type failingExecDB struct {
	postgres.DBTX
	statement string
	failure   error
}

func (db failingExecDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if strings.HasPrefix(query, db.statement) {
		return pgconn.CommandTag{}, db.failure
	}
	return db.DBTX.Exec(ctx, query, args...)
}

func (db failingExecDB) Begin(ctx context.Context) (pgx.Tx, error) {
	base, ok := db.DBTX.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return nil, stderrors.New("underlying database cannot begin transaction")
	}
	tx, err := base.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return failingExecTx{Tx: tx, statement: db.statement, failure: db.failure}, nil
}

type failingExecTx struct {
	pgx.Tx
	statement string
	failure   error
}

func (tx failingExecTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if strings.HasPrefix(query, tx.statement) {
		return pgconn.CommandTag{}, tx.failure
	}
	return tx.Tx.Exec(ctx, query, args...)
}

// rollbackFailureDB makes every transaction rollback fail, to check both causes are kept.
type rollbackFailureDB struct {
	TransactionDB
	failure error
}

func (db rollbackFailureDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.TransactionDB.Begin(ctx)
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

// authFixture is a bootstrapped AuthService whose service and repository databases can be swapped.
type authFixture struct {
	AuthService
	pool         *pgxpool.Pool
	db           *swappableDB
	repositoryDB *swappableDB
	users        repository.UserRepo
	sessions     repository.SessionRepo
}

func newTestService(t *testing.T) (*authFixture, *clock) {
	t.Helper()
	pool := pgtest.Start(t)
	testClock := &clock{t: time.Now()}
	authConfig := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	repositoryDB := &swappableDB{inner: pool}
	users := repository.NewUserRepo(repositoryDB)
	sessions := repository.NewSessionRepo(repositoryDB, 0)
	db := &swappableDB{inner: pool}
	fixture := &authFixture{
		AuthService:  NewAuthService(users, sessions, db, authConfig, testClock.now),
		pool:         pool,
		db:           db,
		repositoryDB: repositoryDB,
		users:        users,
		sessions:     sessions,
	}
	require.NoError(t, fixture.EnsureOwner(tenant.WithID(t.Context(), tenant.PlatformID)))
	return fixture, testClock
}

func codeOf(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}
