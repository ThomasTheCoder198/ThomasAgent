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

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authrepo "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider"
	llmprovidercontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmproviderservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/vault"
)

const (
	providerCatalogBreaker = "provider-catalog"
	postgresDependency     = "postgres"
	redisDependency        = "redis"
	coreListeningMessage   = "core listening"
)

type providerService interface {
	llmproviderservice.LLMProviderService
	llmproviderservice.Resolver
}

type services struct {
	auth     authservice.AuthService
	purger   sessionPurger
	provider providerService
}

type controllers struct {
	auth          *authcontroller.AuthController
	provider      *llmprovidercontroller.ProviderController
	model         *llmprovidercontroller.ModelController
	modelRole     *llmprovidercontroller.ModelRoleController
	internalModel *llmprovidercontroller.InternalModelController
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
	svc, err := rt.newServices(ctx, pool, cipher)
	if err != nil {
		return err
	}
	ctrl, err := rt.newControllers(svc, redisClient)
	if err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	engine := httpx.NewRouter(httpx.NewSlogErrorLogger(rt.log), httpx.LogAccess(rt.log), httpx.RecordMetrics(rt.metrics))
	registerRoutes(engine, ctrl, middlewares{
		auth:           svc.auth,
		serviceToken:   rt.cfg.ServiceToken,
		health:         rt.readinessChecks(pool, redisClient),
		requestTimeout: rt.cfg.HTTP.RequestTimeout,
	})
	go rt.purgeExpiredSessions(ctx, svc.purger)
	go rt.serveMetrics(ctx)
	return rt.listenAndServe(ctx, engine)
}

func (rt *coreRuntime) newServices(ctx context.Context, pool *pgxpool.Pool, cipher *vault.Cipher) (services, error) {
	auth := authservice.NewAuthService(authrepo.NewAuthRepositoryWithSessionTouchInterval(pool, rt.cfg.Auth.SessionTouchInterval), pool, rt.cfg.Auth, time.Now)
	if err := auth.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return services{}, err
	}
	client, err := outbound.NewClient(rt.cfg.ProviderHTTPTimeout, rt.cfg.ProviderPrivateAllowlist)
	if err != nil {
		return services{}, fmt.Errorf("provider HTTP configuration: %w", err)
	}
	breaker := retry.NewBreaker(providerCatalogBreaker, rt.cfg.ProviderBreaker)
	catalog := llmprovider.NewHTTPCatalogWithClient(retry.NewPolicy(rt.cfg.Retry), client, rt.cfg.ProviderMaxResponseBytes, breaker, rt.log, rt.cfg.ProviderMaxBreakers)
	catalog.Metrics = rt.metrics
	provider := llmproviderservice.NewLLMProviderService(pool, vault.NewStore(cipher), catalog, rt.cfg.ProviderMaxRemoteModels)
	return services{
		auth:     auth,
		purger:   auth,
		provider: provider,
	}, nil
}

func (rt *coreRuntime) newControllers(svc services, redisClient *redis.Client) (controllers, error) {
	proxyNetworks, err := parseTrustedProxyCIDRs(rt.cfg.Auth.TrustedProxyCIDRs)
	if err != nil {
		return controllers{}, err
	}
	maxBodyBytes := rt.cfg.HTTP.MaxBodyBytes
	return controllers{
		auth: authcontroller.NewAuthController(authcontroller.AuthControllerParams{
			Service:            svc.auth,
			Limiter:            authlimiter.NewLoginLimiter(redisClient, rt.cfg.Auth, []byte(rt.cfg.ServiceToken)),
			CookieSecure:       rt.cfg.Auth.CookieSecure,
			TrustedProxyHeader: rt.cfg.Auth.TrustedProxyHeader,
			TrustedProxyCIDRs:  proxyNetworks,
			Logger:             rt.log,
		}),
		provider:      llmprovidercontroller.NewProviderController(svc.provider, maxBodyBytes),
		model:         llmprovidercontroller.NewModelController(svc.provider, maxBodyBytes),
		modelRole:     llmprovidercontroller.NewModelRoleController(svc.provider, maxBodyBytes),
		internalModel: llmprovidercontroller.NewInternalModelController(svc.provider),
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

func (rt *coreRuntime) readinessChecks(pool *pgxpool.Pool, redisClient *redis.Client) map[string]httpx.DependencyPinger {
	return map[string]httpx.DependencyPinger{
		postgresDependency: pool,
		redisDependency:    redisx.ClientPinger{Client: redisClient},
	}
}
