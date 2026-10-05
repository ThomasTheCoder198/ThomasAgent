package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestSessionPurgeJobDeletesExpiredRowsAndStopsOnCancellation(t *testing.T) {
	pool := pgtest.Start(t)
	repo := auth.NewRepository(pool)
	user, err := repo.CreateUser(tenant.WithID(t.Context(), tenant.PlatformID), "purge@example.com", "unused-hash")
	require.NoError(t, err)
	require.NoError(t, repo.CreateSession(tenant.WithID(t.Context(), tenant.PlatformID), user.ID, []byte("expired-job-session"), "csrf", time.Now().Add(-time.Hour)))
	service := auth.NewService(repo, pool, config.AuthConfig{}, time.Now)
	app := application{cfg: config.Config{Auth: config.AuthConfig{SessionPurgeInterval: 10 * time.Millisecond}}, log: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() { app.purgeExpiredSessions(ctx, service); close(finished) }()
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
	repo := auth.NewRepository(pool)
	for _, tenantID := range []string{tenant.PlatformID, "other"} {
		ctx := tenant.WithID(t.Context(), tenantID)
		user, err := repo.CreateUser(ctx, "purge@example.com", "unused-hash")
		require.NoError(t, err)
		require.NoError(t, repo.CreateSession(ctx, user.ID, []byte(tenantID+"-expired"), "csrf", time.Now().Add(-time.Hour)))
		require.NoError(t, repo.CreateSession(ctx, user.ID, []byte(tenantID+"-active"), "csrf", time.Now().Add(time.Hour)))
	}
	service := auth.NewService(repo, pool, config.AuthConfig{}, time.Now)
	app := application{log: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	app.runSessionPurge(t.Context(), service)
	var expired, active int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE expires_at<=now()), count(*) FILTER (WHERE expires_at>now()) FROM sessions`).Scan(&expired, &active))
	require.Zero(t, expired, "system cleanup must include every tenant")
	require.Equal(t, 2, active, "system cleanup must preserve active sessions in both tenants")
}
