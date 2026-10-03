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
	redisClient, err := a.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(a.log), httpx.TraceRequests(a.cfg.ServiceName), httpx.LogAccess(a.log))
	httpx.MountHealth(r, a.readinessChecks(pool, redisClient))
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

func (a *application) readinessChecks(pool *pgxpool.Pool, redisClient *redis.Client) map[string]httpx.DependencyPinger {
	return map[string]httpx.DependencyPinger{"postgres": pool, "redis": redisx.ClientPinger{Client: redisClient}}
}
