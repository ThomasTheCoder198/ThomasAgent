package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	testOwnerEmail    = "thomas@example.com"
	testOwnerPassword = "correct horse battery"
)

func TestPasswordHashUsesConfiguredCostAndFixedParallelism(t *testing.T) {
	cfg := config.AuthConfig{PasswordHash: config.PasswordHashConfig{Iterations: 3, MemoryKiB: 65536, MaxConcurrency: 1}}
	s := NewAuthService(nil, nil, cfg, time.Now)
	encoded, err := s.createHash(t.Context(), "test-password")
	require.NoError(t, err)
	require.Contains(t, encoded, "$m=65536,t=3,p=1$")
	matched, err := s.compareHash(t.Context(), "test-password", encoded)
	require.NoError(t, err)
	require.True(t, matched)
}

func TestPasswordHashWaitsForSemaphoreCapacity(t *testing.T) {
	s := NewAuthService(nil, nil, config.AuthConfig{PasswordHash: config.PasswordHashConfig{Iterations: 2, MemoryKiB: 65536, MaxConcurrency: 1}}, time.Now)
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

func TestTimingHashInitializationFailureRejectsAuth(t *testing.T) {
	initializationErr := errors.ErrInternalError.WithCause(context.Canceled)
	service := &Service{decoyErr: initializationErr}
	require.ErrorIs(t, service.EnsureOwner(tenant.WithID(context.Background(), tenant.PlatformID)), initializationErr)
	_, _, err := service.Login(tenant.WithID(context.Background(), tenant.PlatformID), testOwnerEmail, testOwnerPassword)
	require.ErrorIs(t, err, initializationErr)
}

func TestPasswordHashCancelledWaitReturnsRetryableCatalogError(t *testing.T) {
	pool := pgtest.Start(t)
	cfg := config.AuthConfig{OwnerEmail: testOwnerEmail, OwnerPassword: testOwnerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	repo := repository.NewAuthRepository(pool)
	svc := NewAuthService(repo, pool, cfg, time.Now)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	require.NoError(t, svc.EnsureOwner(ctx))
	queried := make(chan struct{})
	repo.DB = notifyQueryDB{DBTX: repo.DB, scanned: queried}
	for range cap(svc.hashSlots) {
		svc.hashSlots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { _, _, err := svc.Login(ctx, testOwnerEmail, "incorrect-password"); result <- err }()
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
