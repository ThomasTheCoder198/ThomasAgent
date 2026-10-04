package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/vault"
)

func (a *application) serveHTTP(ctx context.Context) error {
	cipher, err := vault.NewCipher(a.cfg.Vault.KeyID, a.cfg.Vault.MasterKey)
	if err != nil {
		return fmt.Errorf("CORE_VAULT_MASTER_KEY: %w", err)
	}
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
	router, err := a.buildRouter(ctx, pool, redisClient, cipher)
	if err != nil {
		return err
	}
	return a.listenAndServe(ctx, router)
}

func (a *application) buildRouter(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, cipher *vault.Cipher) (http.Handler, error) {
	authService := auth.NewService(auth.NewRepository(pool), pool, a.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(ctx); err != nil {
		return nil, err
	}
	client, err := outbound.NewClient(a.cfg.ProviderHTTPTimeout, a.cfg.ProviderPrivateAllowlist)
	if err != nil {
		return nil, fmt.Errorf("provider HTTP configuration: %w", err)
	}
	breaker := retry.NewBreaker("provider-catalog", a.cfg.ProviderBreaker)
	catalog := registry.NewHTTPCatalogWithClient(retry.NewPolicy(a.cfg.Retry), client, a.cfg.ProviderMaxResponseBytes, breaker, a.log)
	registryService := registry.NewService(pool, vault.NewStore(cipher), catalog)
	return newRouter(routerDeps{
		log: a.log, serviceName: a.cfg.ServiceName,
		auth:         authService,
		limiter:      auth.NewLimiter(redisClient, a.cfg.Auth.LoginMaxAttempts, a.cfg.Auth.LoginWindow),
		cookieSecure: a.cfg.Auth.CookieSecure,
		registry:     registryService, serviceToken: a.cfg.ServiceToken,
		health:         a.readinessChecks(pool, redisClient),
		requestTimeout: a.cfg.HTTP.RequestTimeout, maxBodyBytes: a.cfg.HTTP.MaxBodyBytes,
	}), nil
}

func (a *application) listenAndServe(ctx context.Context, handler http.Handler) error {
	srv := &http.Server{Addr: a.cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: a.cfg.ReadHeaderTimeout}
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
