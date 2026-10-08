package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llm"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const (
	providerCatalogBreaker = "provider-catalog"
	postgresDependency     = "postgres"
	redisDependency        = "redis"
	coreListeningMessage   = "core listening"
)

type services struct {
	auth *auth.Service
	llm  *llm.Service
}

type controllers struct {
	auth      *auth.Controller
	providers *llm.ProviderController
	models    *llm.ModelController
	roles     *llm.RoleController
	resolve   *llm.ResolveController
}

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
	domainServices, err := rt.newServices(ctx, pool, cipher)
	if err != nil {
		return err
	}
	domainControllers, err := rt.newControllers(domainServices, redisClient)
	if err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	engine := httpx.NewRouter(httpx.NewSlogErrorLogger(rt.log), httpx.LogAccess(rt.log), httpx.RecordMetrics(rt.metrics))
	registerRoutes(engine, domainControllers, middlewares{
		sessions:       domainServices.auth,
		serviceToken:   rt.cfg.ServiceToken,
		health:         rt.readinessChecks(pool, redisClient),
		requestTimeout: rt.cfg.HTTP.RequestTimeout,
	})
	go rt.purgeExpiredSessions(ctx, domainServices.auth)
	go rt.serveMetrics(ctx)
	return rt.listenAndServe(ctx, engine)
}

func (rt *coreRuntime) newServices(ctx context.Context, pool *pgxpool.Pool, cipher *vault.Cipher) (services, error) {
	authService := auth.NewService(auth.NewRepositoryWithSessionTouchInterval(pool, rt.cfg.Auth.SessionTouchInterval), pool, rt.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return services{}, err
	}
	client, err := outbound.NewClient(rt.cfg.ProviderHTTPTimeout, rt.cfg.ProviderPrivateAllowlist)
	if err != nil {
		return services{}, fmt.Errorf("provider HTTP configuration: %w", err)
	}
	breaker := retry.NewBreaker(providerCatalogBreaker, rt.cfg.ProviderBreaker)
	catalog := llm.NewHTTPModelCatalogWithClient(retry.NewPolicy(rt.cfg.Retry), client, rt.cfg.ProviderMaxResponseBytes, breaker, rt.log, rt.cfg.ProviderMaxBreakers)
	catalog.Metrics = rt.metrics
	llmService := llm.NewService(pool, vault.NewStore(cipher), catalog, rt.cfg.ProviderMaxRemoteModels)
	return services{auth: authService, llm: llmService}, nil
}

func (rt *coreRuntime) newControllers(domainServices services, redisClient *redis.Client) (controllers, error) {
	proxyNetworks, err := parseTrustedProxyCIDRs(rt.cfg.Auth.TrustedProxyCIDRs)
	if err != nil {
		return controllers{}, err
	}
	maxBodyBytes := rt.cfg.HTTP.MaxBodyBytes
	return controllers{
		auth: auth.NewController(auth.ControllerParams{
			Service:            domainServices.auth,
			Limiter:            auth.NewLoginLimiter(redisClient, rt.cfg.Auth, []byte(rt.cfg.ServiceToken)),
			CookieSecure:       rt.cfg.Auth.CookieSecure,
			TrustedProxyHeader: rt.cfg.Auth.TrustedProxyHeader,
			TrustedProxyCIDRs:  proxyNetworks,
			Logger:             rt.log,
		}),
		providers: llm.NewProviderController(domainServices.llm, maxBodyBytes),
		models:    llm.NewModelController(domainServices.llm, maxBodyBytes),
		roles:     llm.NewRoleController(domainServices.llm, maxBodyBytes),
		resolve:   llm.NewResolveController(domainServices.llm),
	}, nil
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
