package auth

import (
	"bytes"
	"context"
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func attemptFrom(h http.Handler, email, peer, forwarded string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"wrong-password"}`))
	req = req.WithContext(tenant.WithID(req.Context(), tenant.PlatformID))
	req.RemoteAddr = peer
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", forwarded)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLogin_IgnoresSpoofedForwardingFromUntrustedPeer(t *testing.T) {
	svc, _ := newService(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger))
	NewHandlerWithProxyHeader(svc, NewLimiter(startRedis(t), 1, time.Minute), false, "X-Forwarded-For", logger).Mount(router)
	require.Equal(t, 401, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.1").Code)
	require.Equal(t, 429, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.2").Code)
}

func TestLogin_EmailBudgetBlocksRotatingIPs(t *testing.T) {
	h, _ := newServer(t, 2)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.1:9999", "").Code)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.2:9999", "").Code)
	require.Equal(t, 429, attemptFrom(h, ownerEmail, "192.0.2.3:9999", "").Code)
}

func TestLimiter_RetryAfterUsesRemainingTTL(t *testing.T) {
	client := startRedis(t)
	lim := NewLimiter(client, 1, time.Minute)
	require.NoError(t, lim.Allow(t.Context(), "remaining"))
	require.NoError(t, client.PExpire(t.Context(), limiterKeyPrefix+"remaining", 20*time.Second).Err())
	err := lim.Allow(t.Context(), "remaining")
	require.Equal(t, errors.CodeRateLimited, errors.ToAppError(err).Code)
	require.EqualValues(t, 20, errors.ToAppError(err).Details["retryAfterSeconds"])
	require.NoError(t, client.PExpire(t.Context(), limiterKeyPrefix+"remaining", 7*time.Second).Err())
	err = lim.Allow(t.Context(), "remaining")
	require.EqualValues(t, 7, errors.ToAppError(err).Details["retryAfterSeconds"])
}

func TestFailedLogin_DoesNotWriteUnboundedAuditRows(t *testing.T) {
	svc, _ := newService(t)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	for range 3 {
		_, _, err := svc.Login(ctx, ownerEmail, "incorrect-password")
		require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, err))
	}
	var count int
	require.NoError(t, svc.db.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='auth.login.failed'`).Scan(&count))
	require.Zero(t, count, "ordinary failed attempts must not persist an audit row")
	svc.db = failingExecDB{DBTX: svc.db, statement: "INSERT INTO audit_events", failure: stderrors.New("audit unavailable")}
	_, _, err := svc.Login(ctx, ownerEmail, "incorrect-password")
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, err))
}

func TestPasswordHash_CancelledWaitReturnsRetryableCatalogError(t *testing.T) {
	svc, _ := newService(t)
	queried := make(chan struct{})
	svc.repo.db = notifyQueryDB{DBTX: svc.repo.db, scanned: queried}
	for range cap(svc.hashSlots) {
		svc.hashSlots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(tenant.WithID(t.Context(), tenant.PlatformID))
	result := make(chan error, 1)
	go func() { _, _, err := svc.Login(ctx, ownerEmail, "incorrect-password"); result <- err }()
	<-queried
	cancel()
	var err error
	select {
	case err = <-result:
	case <-time.After(250 * time.Millisecond):
		<-svc.hashSlots
		<-result
		t.Fatal("password hash ignores canceled context while waiting for capacity")
	}
	for len(svc.hashSlots) > 0 {
		<-svc.hashSlots
	}
	require.ErrorIs(t, err, context.Canceled)
	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	require.True(t, appErr.Retryable())
}

type notifyQueryDB struct {
	postgres.DBTX
	scanned chan struct{}
}

func (db notifyQueryDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return notifyRow{Row: db.DBTX.QueryRow(ctx, sql, args...), scanned: db.scanned}
}

type notifyRow struct {
	pgx.Row
	scanned chan struct{}
}

func (row notifyRow) Scan(args ...any) error {
	err := row.Row.Scan(args...)
	close(row.scanned)
	return err
}

type scriptExpiryFailureHook struct{ failure error }

func (h scriptExpiryFailureHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h scriptExpiryFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h scriptExpiryFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "evalsha" || cmd.Name() == "eval" {
			return h.failure
		}
		return next(ctx, cmd)
	}
}
