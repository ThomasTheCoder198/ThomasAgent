package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestEnqueueOutbox_RolledBackIsNeverPublished(t *testing.T) {
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, enqueueTestOutbox(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Rollback(ctx))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestEnqueueOutboxUsesTrustedTenantContext(t *testing.T) {
	pool := startPool(t)
	ctx := tenant.WithID(t.Context(), "other")
	require.NoError(t, enqueueTestOutbox(ctx, pool, testStream, map[string]string{"file_id": "other-file"}))
	var scope string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tenant_id FROM outbox`).Scan(&scope))
	require.Equal(t, "other", scope)
	require.Error(t, enqueueTestOutbox(t.Context(), pool, testStream, map[string]string{"file_id": "missing"}))
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestEnqueueOutbox_StoresRequestIDOrJobID(t *testing.T) {
	pool := startPool(t)
	base := tenant.WithID(t.Context(), tenant.PlatformID)
	const origin = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	require.NoError(t, enqueueTestOutbox(requestid.NewContext(base, origin), pool, testStream, map[string]string{"n": "1"}))
	require.NoError(t, enqueueTestOutbox(base, pool, testStream, map[string]string{"n": "2"}))
	require.NoError(t, enqueueTestOutbox(requestid.NewContext(base, "forged id"), pool, testStream, map[string]string{"n": "3"}))
	rows, err := pool.Query(base, `SELECT request_id FROM outbox ORDER BY payload->>'n'`)
	require.NoError(t, err)
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, ids, 3)
	require.Equal(t, origin, ids[0])
	require.True(t, strings.HasPrefix(ids[1], requestid.JobPrefix), ids[1])
	require.True(t, strings.HasPrefix(ids[2], requestid.JobPrefix), ids[2])
}
