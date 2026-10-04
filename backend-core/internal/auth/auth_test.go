package auth

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

const (
	ownerEmail    = "thomas@example.com"
	ownerPassword = "correct horse battery"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService(t *testing.T) (*Service, *clock) {
	t.Helper()
	pool := pgtest.Start(t)
	c := &clock{t: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)}
	cfg := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	s := NewService(NewRepository(pool), pool, cfg, c.now)
	require.NoError(t, s.EnsureOwner(context.Background()))
	return s, c
}

func codeOf(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}

func TestEnsureOwnerIsIdempotent(t *testing.T) {
	s, _ := newService(t)
	require.NoError(t, s.EnsureOwner(context.Background()))
	_, _, err := s.Login(context.Background(), ownerEmail, ownerPassword)
	require.NoError(t, err)
}

func TestEnsureOwnerRejectsShortPassword(t *testing.T) {
	pool := pgtest.Start(t)
	cfg := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: "short", SessionTTL: time.Hour, MinPasswordLength: 12}
	err := NewService(NewRepository(pool), pool, cfg, time.Now).EnsureOwner(context.Background())
	require.ErrorContains(t, err, "CORE_AUTH_OWNER_PASSWORD")
}

func TestLoginIssuesSessionThatAuthenticates(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	user, issued, err := s.Login(ctx, "  Thomas@Example.com ", ownerPassword)
	require.NoError(t, err)
	require.Equal(t, ownerEmail, user.Email)
	require.NotEmpty(t, issued.Token)
	require.NotEmpty(t, issued.CSRFToken)

	got, sess, err := s.Authenticate(ctx, issued.Token)
	require.NoError(t, err)
	require.Equal(t, user.ID, got.ID)
	require.Equal(t, issued.CSRFToken, sess.CSRFToken)
}

func TestLoginErrorsAreUniform(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_, _, errWrongPass := s.Login(ctx, ownerEmail, "wrong password!!")
	_, _, errUnknown := s.Login(ctx, "nobody@example.com", ownerPassword)
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, errWrongPass))
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, errUnknown))
}

func TestExpiredSessionIsRejected(t *testing.T) {
	s, c := newService(t)
	ctx := context.Background()
	_, issued, err := s.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	c.t = c.t.Add(2 * time.Hour)
	_, _, err = s.Authenticate(ctx, issued.Token)
	require.Equal(t, errors.CodeAuthSessionExpired, codeOf(t, err))
}

func TestLogoutInvalidatesSession(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_, issued, _ := s.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, s.Logout(ctx, issued.Token))
	_, _, err := s.Authenticate(ctx, issued.Token)
	require.Equal(t, errors.CodeUnauthenticated, codeOf(t, err))
}

func TestTimingHashInitializationFailureRejectsAuth(t *testing.T) {
	initializationErr := stderrors.New("test timing hash initialization failure")
	service := &Service{decoyErr: initializationErr}
	require.ErrorIs(t, service.EnsureOwner(context.Background()), initializationErr)
	_, _, err := service.Login(context.Background(), ownerEmail, ownerPassword)
	require.ErrorIs(t, err, initializationErr)
}

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
func TestLoginAuditFailureRejectsAndCleansSession(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()
	auditErr := stderrors.New("test audit insert failure")
	service.db = failingExecDB{DBTX: service.db, statement: "INSERT INTO audit_events", failure: auditErr}
	user, issued, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.ErrorIs(t, err, auditErr)
	require.Equal(t, User{}, user)
	require.Equal(t, IssuedSession{}, issued)
	var count int
	require.NoError(t, service.repo.db.QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&count))
	require.Zero(t, count)
	require.NotContains(t, err.Error(), ownerPassword)
}
func TestLoginAuditAndCleanupFailuresAreRetained(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()
	auditErr := stderrors.New("test audit insert failure")
	cleanupErr := stderrors.New("test session deletion failure")
	service.db = failingExecDB{DBTX: service.db, statement: "INSERT INTO audit_events", failure: auditErr}
	service.repo.db = failingExecDB{DBTX: service.repo.db, statement: "DELETE FROM sessions", failure: cleanupErr}
	user, issued, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.ErrorIs(t, err, auditErr)
	require.ErrorIs(t, err, cleanupErr)
	require.Equal(t, User{}, user)
	require.Equal(t, IssuedSession{}, issued)
	require.NotContains(t, err.Error(), ownerPassword)
}
func TestExpiredSessionCleanupFailureReturnsInternalErrorAndCause(t *testing.T) {
	service, clock := newService(t)
	ctx := context.Background()
	_, issued, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	cleanupErr := stderrors.New("test expired session deletion failure")
	service.repo.db = failingExecDB{DBTX: service.repo.db, statement: "DELETE FROM sessions", failure: cleanupErr}
	clock.t = clock.t.Add(2 * time.Hour)
	user, session, err := service.Authenticate(ctx, issued.Token)
	require.ErrorIs(t, err, cleanupErr)
	require.Equal(t, errors.CodeInternalError, codeOf(t, err))
	require.Equal(t, User{}, user)
	require.Equal(t, Session{}, session)
	require.NotContains(t, err.Error(), issued.Token)
	require.NotContains(t, err.Error(), issued.CSRFToken)
	require.NotContains(t, err.Error(), ownerPassword)
}
