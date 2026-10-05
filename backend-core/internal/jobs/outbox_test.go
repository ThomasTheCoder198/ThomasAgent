package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestEnqueueOutbox_RolledBackIsNeverPublished(t *testing.T) {
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, EnqueueOutbox(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Rollback(ctx))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestEnqueueOutboxUsesTrustedTenantContext(t *testing.T) {
	pool := startPool(t)
	ctx := tenant.WithID(t.Context(), "other")
	require.NoError(t, EnqueueOutbox(ctx, pool, testStream, map[string]string{"file_id": "other-file"}))
	var scope string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tenant_id FROM outbox`).Scan(&scope))
	require.Equal(t, "other", scope)
	require.Error(t, EnqueueOutbox(t.Context(), pool, testStream, map[string]string{"file_id": "missing"}))
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&count))
	require.Equal(t, 1, count)
}
