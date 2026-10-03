package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
)

func (a *application) serveHTTP(ctx context.Context) error {
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
	r := httpx.NewRouter(httpx.SlogErrorLogger(a.log), httpx.Tracing(a.cfg.ServiceName), httpx.AccessLog(a.log))
	httpx.MountHealth(r, a.readinessChecks(pool, rdb))
	srv := &http.Server{Addr: a.cfg.HTTPAddr, Handler: r, ReadHeaderTimeout: a.cfg.ReadHeaderTimeout}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	a.log.InfoContext(ctx, "core listening", "addr", a.cfg.HTTPAddr)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func (a *application) readinessChecks(pool *pgxpool.Pool, rdb *redis.Client) map[string]httpx.Pinger {
	return map[string]httpx.Pinger{"postgres": pool, "redis": redisx.Pinger{Client: rdb}}
}
