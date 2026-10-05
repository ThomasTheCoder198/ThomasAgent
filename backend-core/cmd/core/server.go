package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
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
	go a.purgeExpiredSessions(ctx, a.authService)
	return a.listenAndServe(ctx, router)
}

func (a *application) buildRouter(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, cipher *vault.Cipher) (http.Handler, error) {
	authService := auth.NewService(auth.NewRepositoryWithSessionTouchInterval(pool, a.cfg.Auth.SessionTouchInterval), pool, a.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return nil, err
	}
	a.authService = authService
	client, err := outbound.NewClient(a.cfg.ProviderHTTPTimeout, a.cfg.ProviderPrivateAllowlist)
	if err != nil {
		return nil, fmt.Errorf("provider HTTP configuration: %w", err)
	}
	breaker := retry.NewBreaker("provider-catalog", a.cfg.ProviderBreaker)
	catalog := registry.NewHTTPCatalogWithClient(retry.NewPolicy(a.cfg.Retry), client, a.cfg.ProviderMaxResponseBytes, breaker, a.log, a.cfg.ProviderMaxBreakers)
	registryService := registry.NewService(pool, vault.NewStore(cipher), catalog, a.cfg.ProviderMaxRemoteModels)
	var proxyNetworks []netip.Prefix
	for _, cidr := range a.cfg.Auth.TrustedProxyCIDRs {
		network, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("CORE_AUTH_TRUSTED_PROXY_CIDRS: %w", err)
		}
		proxyNetworks = append(proxyNetworks, network)
	}
	return newRouter(routerDeps{
		log: a.log, serviceName: a.cfg.ServiceName,
		auth:               authService,
		limiter:            auth.NewLoginLimiter(redisClient, a.cfg.Auth, []byte(a.cfg.ServiceToken)),
		cookieSecure:       a.cfg.Auth.CookieSecure,
		trustedProxyHeader: a.cfg.Auth.TrustedProxyHeader,
		trustedProxyCIDRs:  proxyNetworks,
		registry:           registryService, serviceToken: a.cfg.ServiceToken,
		health:         a.readinessChecks(pool, redisClient),
		requestTimeout: a.cfg.HTTP.RequestTimeout, maxBodyBytes: a.cfg.HTTP.MaxBodyBytes,
	}), nil
}

func (a *application) purgeExpiredSessions(ctx context.Context, service *auth.Service) {
	if a.cfg.Auth.SessionPurgeInterval <= 0 {
		return
	}
	ticker := time.NewTicker(a.cfg.Auth.SessionPurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runSessionPurge(ctx, service)
		}
	}
}

func (a *application) runSessionPurge(ctx context.Context, service *auth.Service) {
	ctx, span := otel.Tracer(auth.TracerName).Start(ctx, auth.SessionPurgeSpan)
	defer span.End()
	count, err := service.PurgeAllExpiredSessions(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "session purge failed")
		a.log.ErrorContext(ctx, "expired session purge failed", "error", err)
		return
	}
	a.log.InfoContext(ctx, "expired sessions purged", "count", count)
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
