package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestSessionPurgeJobDeletesExpiredRowsAndStopsOnCancellation(t *testing.T) {
	pool := pgtest.Start(t)
	users := authrepository.NewUserRepo(pool)
	sessions := authrepository.NewSessionRepo(pool, 0)
	user, err := users.CreateUser(tenant.WithID(t.Context(), tenant.PlatformID), "purge@example.com", "unused-hash")
	require.NoError(t, err)
	require.NoError(t, sessions.CreateSession(tenant.WithID(t.Context(), tenant.PlatformID), user.ID, []byte("expired-job-session"), "csrf", time.Now().Add(-time.Hour)))
	service := authservice.NewAuthService(users, sessions, pool, config.AuthConfig{}, time.Now)
	app := coreRuntime{cfg: config.Config{Auth: config.AuthConfig{SessionPurgeInterval: 10 * time.Millisecond}, SessionPurgeTimeout: time.Second}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	runner := periodic.Runner{Redis: startWorkerTestRedis(t), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	go func() { _ = runner.Run(ctx, []periodic.Job{app.sessionPurgeJob(service)}); close(finished) }()
	require.Eventually(t, func() bool {
		var count int
		err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions`).Scan(&count)
		return err == nil && count == 0
	}, time.Second, 10*time.Millisecond, "the configured purge job must delete expired sessions")
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("purge job did not stop on cancellation")
	}
}

func TestSessionPurgeJobDeletesExpiredSessionsAcrossTenants(t *testing.T) {
	pool := pgtest.Start(t)
	users := authrepository.NewUserRepo(pool)
	sessions := authrepository.NewSessionRepo(pool, 0)
	for _, tenantID := range []string{tenant.PlatformID, "other"} {
		ctx := tenant.WithID(t.Context(), tenantID)
		user, err := users.CreateUser(ctx, "purge@example.com", "unused-hash")
		require.NoError(t, err)
		require.NoError(t, sessions.CreateSession(ctx, user.ID, []byte(tenantID+"-expired"), "csrf", time.Now().Add(-time.Hour)))
		require.NoError(t, sessions.CreateSession(ctx, user.ID, []byte(tenantID+"-active"), "csrf", time.Now().Add(time.Hour)))
	}
	service := authservice.NewAuthService(users, sessions, pool, config.AuthConfig{}, time.Now)
	app := coreRuntime{}
	_, err := app.sessionPurgeJob(service).Run(t.Context())
	require.NoError(t, err)
	var expired, active int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE expires_at<=now()), count(*) FILTER (WHERE expires_at>now()) FROM sessions`).Scan(&expired, &active))
	require.Zero(t, expired, "system cleanup must include every tenant")
	require.Equal(t, 2, active, "system cleanup must preserve active sessions in both tenants")
}

func TestSessionPurgeJobHasConfiguredTimeout(t *testing.T) {
	app := coreRuntime{cfg: config.Config{SessionPurgeTimeout: 2 * time.Minute}}
	job := app.sessionPurgeJob(fakeSessionPurger{})
	require.Equal(t, 2*time.Minute, job.Timeout)
}

func TestSessionPurgeLogsCarryJobRequestID(t *testing.T) {
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	app := coreRuntime{cfg: config.Config{Auth: config.AuthConfig{SessionPurgeInterval: time.Millisecond}, SessionPurgeTimeout: time.Second}}
	runner := periodic.Runner{Redis: startWorkerTestRedis(t), Log: log}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { _ = runner.Run(ctx, []periodic.Job{app.sessionPurgeJob(fakeSessionPurger{})}); close(done) }()
	require.Eventually(t, func() bool { return strings.Contains(output.String(), "periodic job completed") }, time.Second, 10*time.Millisecond)
	cancel()
	<-done
	require.Contains(t, output.String(), `"request_id":"`+requestid.JobPrefix)
	require.Contains(t, output.String(), `"duration_ms":`)
}

type fakeSessionPurger struct{}

func (fakeSessionPurger) PurgeExpiredSessionsAcrossTenants(context.Context) (int64, error) {
	return 0, nil
}
