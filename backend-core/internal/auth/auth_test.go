package auth

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"

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
	c := &clock{t: time.Now()}
	cfg := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	s := NewService(NewRepository(pool), pool, cfg, c.now)
	require.NoError(t, s.EnsureOwner(tenant.WithID(context.Background(), tenant.PlatformID)))
	return s, c
}

func codeOf(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}

func TestEnsureOwnerIsIdempotent(t *testing.T) {
	s, _ := newService(t)
	require.NoError(t, s.EnsureOwner(tenant.WithID(context.Background(), tenant.PlatformID)))
	_, _, err := s.Login(tenant.WithID(context.Background(), tenant.PlatformID), ownerEmail, ownerPassword)
	require.NoError(t, err)
}

func TestEnsureOwnerConcurrentDifferentEmailsCreatesOneOwner(t *testing.T) {
	pool := pgtest.Start(t)
	services := make([]*Service, 4)
	for i := range services {
		cfg := config.AuthConfig{OwnerEmail: uuid.NewString() + "@example.com", OwnerPassword: ownerPassword, MinPasswordLength: 12}
		services[i] = NewService(NewRepository(pool), pool, cfg, time.Now)
	}
	start := make(chan struct{})
	results := make(chan error, len(services))
	var group sync.WaitGroup
	for _, service := range services {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- service.EnsureOwner(tenant.WithID(t.Context(), tenant.PlatformID))
		}()
	}
	close(start)
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	count, err := services[0].repo.CountUsers(tenant.WithID(t.Context(), tenant.PlatformID))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestAuthRepositoryUsesTrustedTenantContext(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewRepository(pool)
	platform, err := repo.CreateUser(tenant.WithID(t.Context(), tenant.PlatformID), ownerEmail, "platform-hash")
	require.NoError(t, err)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := repo.CreateUser(otherCtx, ownerEmail, "other-hash")
	require.NoError(t, err)
	require.NotEqual(t, platform.ID, other.ID)
	got, hash, err := repo.FindUserByEmail(otherCtx, ownerEmail)
	require.NoError(t, err)
	require.Equal(t, other.ID, got.ID)
	require.Equal(t, "other-hash", hash)
	require.NoError(t, repo.CreateSession(otherCtx, other.ID, []byte("other-token"), "csrf", time.Now().Add(time.Hour)))
	_, _, err = repo.FindSession(tenant.WithID(t.Context(), tenant.PlatformID), []byte("other-token"))
	require.ErrorIs(t, err, errNotFound)
	_, _, err = repo.FindSession(otherCtx, []byte("other-token"))
	require.NoError(t, err)
	_, err = repo.db.Exec(tenant.WithID(t.Context(), tenant.PlatformID), `INSERT INTO users (tenant_id,email,password_hash) VALUES ('other','second@example.com','hash')`)
	require.NoError(t, err)
	count, err := repo.CountUsers(otherCtx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestEnsureOwnerRejectsShortPassword(t *testing.T) {
	pool := pgtest.Start(t)
	cfg := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: "short", SessionTTL: time.Hour, MinPasswordLength: 12}
	err := NewService(NewRepository(pool), pool, cfg, time.Now).EnsureOwner(tenant.WithID(context.Background(), tenant.PlatformID))
	require.ErrorContains(t, err, "CORE_AUTH_OWNER_PASSWORD")
}

func TestLoginIssuesSessionThatAuthenticates(t *testing.T) {
	s, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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

func TestFindUserByEmailScopesSameEmailByTenant(t *testing.T) {
	service, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	otherID := uuid.New()
	_, err := service.repo.db.Exec(ctx, `INSERT INTO users (id, tenant_id, email, password_hash) VALUES ($1, $2, $3, $4)`, otherID, "other", ownerEmail, "other-hash")
	require.NoError(t, err)
	user, _, err := service.repo.FindUserByEmail(ctx, ownerEmail)
	require.NoError(t, err)
	require.NotEqual(t, otherID, user.ID)
	count, err := service.repo.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestLoginErrorsAreUniform(t *testing.T) {
	s, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, _, errWrongPass := s.Login(ctx, ownerEmail, "wrong password!!")
	_, _, errUnknown := s.Login(ctx, "nobody@example.com", ownerPassword)
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, errWrongPass))
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, errUnknown))
}

func TestFailedLoginAuditContainsNoCredentials(t *testing.T) {
	service, _ := newService(t)
	_, _, err := service.Login(tenant.WithID(context.Background(), tenant.PlatformID), ownerEmail, "incorrect password")
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, err))
	require.NoError(t, service.recordFailedLogin(tenant.WithID(t.Context(), tenant.PlatformID)))
	var targetID string
	var metadata []byte
	require.NoError(t, service.db.QueryRow(tenant.WithID(context.Background(), tenant.PlatformID), `SELECT target_id, metadata FROM audit_events WHERE action = 'auth.login.failed'`).Scan(&targetID, &metadata))
	require.Empty(t, targetID)
	require.JSONEq(t, `{}`, string(metadata))
	require.NotContains(t, string(metadata), "incorrect password")
}

func TestExpiredSessionIsRejected(t *testing.T) {
	s, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, issued, err := s.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	_, err = s.db.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hashToken(issued.Token))
	require.NoError(t, err)
	_, _, err = s.Authenticate(ctx, issued.Token)
	require.Equal(t, errors.CodeAuthSessionExpired, codeOf(t, err))
}

func TestAuthenticateRefreshesLastSeen(t *testing.T) {
	service, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, issued, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE sessions SET last_seen_at = '2000-01-01' WHERE token_hash = $1`, hashToken(issued.Token))
	require.NoError(t, err)
	_, _, err = service.Authenticate(ctx, issued.Token)
	require.NoError(t, err)
	var lastSeen time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash = $1`, hashToken(issued.Token)).Scan(&lastSeen))
	require.True(t, lastSeen.After(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func TestPurgeExpiredSessionsOnlyDeletesExpiredPlatformSessions(t *testing.T) {
	service, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	user, _, err := service.repo.FindUserByEmail(ctx, ownerEmail)
	require.NoError(t, err)
	require.NoError(t, service.repo.CreateSession(ctx, user.ID, []byte("expired-platform-session-hash"), "csrf", time.Now().Add(-time.Hour)))
	_, err = service.db.Exec(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ('other', 'other@example.com', 'hash')`)
	require.NoError(t, err)
	deleted, err := service.PurgeExpiredSessions(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
}

func TestLogoutInvalidatesSession(t *testing.T) {
	s, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, issued, _ := s.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, s.Logout(ctx, issued.Token))
	_, _, err := s.Authenticate(ctx, issued.Token)
	require.Equal(t, errors.CodeUnauthenticated, codeOf(t, err))
}

func TestTimingHashInitializationFailureRejectsAuth(t *testing.T) {
	initializationErr := stderrors.New("test timing hash initialization failure")
	service := &Service{decoyErr: initializationErr}
	require.ErrorIs(t, service.EnsureOwner(tenant.WithID(context.Background(), tenant.PlatformID)), initializationErr)
	_, _, err := service.Login(tenant.WithID(context.Background(), tenant.PlatformID), ownerEmail, ownerPassword)
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

func TestLoginAuditFailureRollsBackWithoutCompensatingDelete(t *testing.T) {
	service, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, err := service.db.Exec(ctx, `CREATE FUNCTION reject_session_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'rollback must not issue DELETE'; END $$;
		CREATE TRIGGER reject_session_delete BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION reject_session_delete()`)
	require.NoError(t, err)
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

func TestPasswordHashUsesConfiguredCostAndFixedParallelism(t *testing.T) {
	cfg := config.AuthConfig{PasswordHash: config.PasswordHashConfig{Iterations: 3, MemoryKiB: 65536, MaxConcurrency: 1}}
	s := NewService(nil, nil, cfg, time.Now)
	encoded, err := s.createHash(t.Context(), "test-password")
	require.NoError(t, err)
	require.Contains(t, encoded, "$m=65536,t=3,p=1$")
	matched, err := s.compareHash(t.Context(), "test-password", encoded)
	require.NoError(t, err)
	require.True(t, matched)
}

func TestPasswordHashWaitsForSemaphoreCapacity(t *testing.T) {
	s := NewService(nil, nil, config.AuthConfig{PasswordHash: config.PasswordHashConfig{Iterations: 2, MemoryKiB: 65536, MaxConcurrency: 1}}, time.Now)
	s.hashSlots <- struct{}{}
	done := make(chan error, 1)
	go func() { _, err := s.createHash(t.Context(), "test-password"); done <- err }()
	select {
	case <-done:
		t.Fatal("hash ran while the configured concurrency budget was occupied")
	case <-time.After(time.Second):
	}
	<-s.hashSlots
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("hash did not resume after a slot was released")
	}
	require.Empty(t, s.hashSlots)
}
func TestExpiredSessionCleanupFailureReturnsInternalErrorAndCause(t *testing.T) {
	service, _ := newService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	_, issued, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hashToken(issued.Token))
	require.NoError(t, err)
	cleanupErr := stderrors.New("test expired session deletion failure")
	service.repo.db = failingExecDB{DBTX: service.repo.db, statement: "DELETE FROM sessions", failure: cleanupErr}
	user, session, err := service.Authenticate(ctx, issued.Token)
	require.ErrorIs(t, err, cleanupErr)
	require.Equal(t, errors.CodeInternalError, codeOf(t, err))
	require.Equal(t, User{}, user)
	require.Equal(t, Session{}, session)
	require.NotContains(t, err.Error(), issued.Token)
	require.NotContains(t, err.Error(), issued.CSRFToken)
	require.NotContains(t, err.Error(), ownerPassword)
}

func TestLoginAuditFailureRejectsAndCleansSession(t *testing.T) {
	TestLoginAuditFailureRollsBackWithoutCompensatingDelete(t)
}

func TestLoginAuditAndCleanupFailuresAreRetained(t *testing.T) {
	svc, _ := newService(t)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	auditErr := stderrors.New("audit write failed")
	cleanupErr := stderrors.New("transaction cleanup failed")
	svc.db = rollbackFailureDB{transactionDB: failingExecDB{DBTX: svc.db, statement: "INSERT INTO audit_events", failure: auditErr}, failure: cleanupErr}
	user, issued, err := svc.Login(ctx, ownerEmail, ownerPassword)
	require.ErrorIs(t, err, auditErr)
	require.ErrorIs(t, err, cleanupErr)
	require.Equal(t, User{}, user)
	require.Equal(t, IssuedSession{}, issued)
	var count int
	require.NoError(t, svc.repo.db.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&count))
	require.Zero(t, count)
	require.NotContains(t, err.Error(), ownerPassword)
}
