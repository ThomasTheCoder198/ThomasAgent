package main

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/jobs"
)

func (a *application) runOutboxRelay(ctx context.Context) error {
	pool, err := a.openPostgres(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, err := a.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	r := &jobs.Relay{Pool: pool, Redis: rdb, BatchSize: a.cfg.Relay.BatchSize, PollInterval: a.cfg.Relay.PollInterval, Log: a.log}
	a.log.InfoContext(ctx, "outbox relay started")
	return r.Run(ctx)
}
