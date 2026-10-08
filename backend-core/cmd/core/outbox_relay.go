package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const outboxRelayStartedMessage = "outbox relay started"

func (rt *coreRuntime) runOutboxRelay(ctx context.Context) error {
	ctx = tenant.WithID(ctx, tenant.PlatformID)
	pool, err := rt.openPostgres(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	redisClient, err := rt.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	r := &jobs.OutboxRelay{
		Pool:         pool,
		Redis:        redisClient,
		BatchSize:    rt.cfg.OutboxRelay.BatchSize,
		PollInterval: rt.cfg.OutboxRelay.PollInterval,
		Log:          rt.log,
		Metrics:      rt.metrics,
	}
	go rt.serveMetrics(ctx)
	rt.log.InfoContext(ctx, outboxRelayStartedMessage)
	return r.Run(ctx)
}
