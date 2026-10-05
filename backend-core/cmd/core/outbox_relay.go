package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func (a *application) runOutboxRelay(ctx context.Context) error {
	ctx = tenant.WithID(ctx, tenant.PlatformID)
	pool, err := a.openPostgres(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	redisClient, err := a.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	r := &jobs.OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: a.cfg.OutboxRelay.BatchSize, PollInterval: a.cfg.OutboxRelay.PollInterval, Log: a.log}
	a.log.InfoContext(ctx, "outbox relay started")
	return r.Run(ctx)
}
