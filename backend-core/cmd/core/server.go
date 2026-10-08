package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/netip"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const (
	postgresDependency   = "postgres"
	redisDependency      = "redis"
	coreListeningMessage = "core listening"
)

func (rt *coreRuntime) serveHTTP(ctx context.Context) error {
	cipher, err := vault.NewCipher(rt.cfg.Vault.KeyID, rt.cfg.Vault.MasterKey)
	if err != nil {
		return fmt.Errorf("CORE_VAULT_MASTER_KEY: %w", err)
	}
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
	clients, err := rt.initClientContainer()
	if err != nil {
		return err
	}
	repositories := rt.initRepositoryContainer(pool)
	services, err := rt.initServiceContainer(ctx, pool, cipher, clients, repositories)
	if err != nil {
		return err
	}
	controllers, err := rt.initControllerContainer(services, redisClient)
	if err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	engine := httpx.NewRouter(httpx.NewSlogErrorLogger(rt.log), httpx.LogAccess(rt.log), httpx.RecordMetrics(rt.metrics))
	registerRoutes(engine, controllers, middlewares{
		authService:    services.AuthService,
		cookieSecure:   rt.cfg.Auth.CookieSecure,
		serviceToken:   rt.cfg.ServiceToken,
		health:         rt.readinessChecks(pool, redisClient),
		requestTimeout: rt.cfg.HTTP.RequestTimeout,
	})
	go rt.purgeExpiredSessions(ctx, services.AuthService)
	go rt.serveMetrics(ctx)
	return rt.listenAndServe(ctx, engine)
}

func parseTrustedProxyCIDRs(cidrs []string) ([]netip.Prefix, error) {
	networks := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		network, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("CORE_AUTH_TRUSTED_PROXY_CIDRS: %w", err)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func (rt *coreRuntime) listenAndServe(ctx context.Context, handler http.Handler) error {
	srv := &http.Server{
		Addr:              rt.cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: rt.cfg.ReadHeaderTimeout,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	rt.log.InfoContext(ctx, coreListeningMessage, logging.FieldAddr, rt.cfg.HTTPAddr)
	select {
	case err := <-errCh:
		if stderrors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), rt.cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func (rt *coreRuntime) readinessChecks(pool *pgxpool.Pool, redisClient *redis.Client) map[string]httpx.ReadinessDependency {
	return map[string]httpx.ReadinessDependency{
		postgresDependency: pool,
		redisDependency:    redisx.ReadinessProbe{Client: redisClient},
	}
}
