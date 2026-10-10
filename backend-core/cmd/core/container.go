package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	llmclient "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/client"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	llmservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

// The containers below are the composition root, one per layer, built in this order:
// clients → repositories → services → controllers.

const providerCatalogBreaker = "provider-catalog"

type ClientContainer struct {
	ModelCatalog *llmclient.HTTPModelCatalog
	Ragx         *ragx.Caller
}

func (rt *coreRuntime) initClientContainer(signingKey ed25519.PrivateKey) (ClientContainer, error) {
	httpClient, err := outbound.NewClient(rt.cfg.ProviderHTTPTimeout, rt.cfg.ProviderPrivateAllowlist)
	if err != nil {
		return ClientContainer{}, fmt.Errorf("provider HTTP configuration: %w", err)
	}
	breaker := retry.NewBreaker(providerCatalogBreaker, rt.cfg.ProviderBreaker)
	modelCatalog := llmclient.NewHTTPModelCatalogWithClient(retry.NewPolicy(rt.cfg.Retry), httpClient, rt.cfg.ProviderMaxResponseBytes, breaker, rt.log, rt.cfg.ProviderMaxBreakers)
	modelCatalog.Metrics = rt.metrics
	var recorder outbound.Recorder
	if rt.metrics != nil { // a nil *Metrics inside the interface would panic on the first call
		recorder = rt.metrics
	}
	signer := apptoken.NewAppTokenSigner(signingKey, rt.cfg.App, time.Now, uuid.NewString)
	ragxCaller, err := ragx.NewRagxCaller(rt.cfg.Ragx, signer, recorder, rt.log)
	if err != nil {
		return ClientContainer{}, fmt.Errorf("ragx client: %w", err)
	}
	return ClientContainer{ModelCatalog: modelCatalog, Ragx: ragxCaller}, nil
}

type RepositoryContainer struct {
	UserRepo     authrepository.UserRepo
	SessionRepo  authrepository.SessionRepo
	ProviderRepo llmrepository.ProviderRepo
	LLMModelRepo llmrepository.LLMModelRepo
	RoleRepo     llmrepository.RoleRepo
}

func (rt *coreRuntime) initRepositoryContainer(pool *pgxpool.Pool) RepositoryContainer {
	providerRepo := llmrepository.NewProviderRepo()
	return RepositoryContainer{
		UserRepo:     authrepository.NewUserRepo(pool),
		SessionRepo:  authrepository.NewSessionRepo(pool, rt.cfg.Auth.SessionTouchInterval),
		ProviderRepo: providerRepo,
		LLMModelRepo: llmrepository.NewLLMModelRepo(providerRepo),
		RoleRepo:     llmrepository.NewRoleRepo(),
	}
}

type ServiceContainer struct {
	AuthService      authservice.AuthService
	ProviderService  llmservice.ProviderService
	LLMModelService  llmservice.LLMModelService
	ModelSyncService llmservice.ModelSyncService
	RoleService      llmservice.RoleService
}

// initServiceContainer also bootstraps the owner account, so startup fails before serving if it cannot.
func (rt *coreRuntime) initServiceContainer(ctx context.Context, pool *pgxpool.Pool, cipher *vault.Cipher, clients ClientContainer, repositories RepositoryContainer) (ServiceContainer, error) {
	authService := authservice.NewAuthService(repositories.UserRepo, repositories.SessionRepo, pool, rt.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return ServiceContainer{}, err
	}
	providerService := llmservice.NewProviderService(pool, vault.NewStore(cipher), repositories.ProviderRepo, clients.ModelCatalog)
	return ServiceContainer{
		AuthService:      authService,
		ProviderService:  providerService,
		LLMModelService:  llmservice.NewLLMModelService(pool, repositories.LLMModelRepo),
		ModelSyncService: llmservice.NewModelSyncService(pool, providerService, repositories.LLMModelRepo, clients.ModelCatalog, rt.cfg.ProviderMaxRemoteModels),
		RoleService:      llmservice.NewRoleService(pool, repositories.LLMModelRepo, repositories.RoleRepo),
	}, nil
}

type ControllerContainer struct {
	AuthController     *authcontroller.AuthController
	ProviderController *llmcontroller.ProviderController
	LLMModelController *llmcontroller.LLMModelController
	RoleController     *llmcontroller.RoleController
}

func (rt *coreRuntime) initControllerContainer(services ServiceContainer, redisClient *redis.Client) (ControllerContainer, error) {
	proxyNetworks, err := parseTrustedProxyCIDRs(rt.cfg.Auth.TrustedProxyCIDRs)
	if err != nil {
		return ControllerContainer{}, err
	}
	maxBodyBytes := rt.cfg.HTTP.MaxBodyBytes
	return ControllerContainer{
		AuthController: authcontroller.NewAuthController(authcontroller.AuthControllerParams{
			AuthService:        services.AuthService,
			Limiter:            authlimiter.NewLoginLimiter(redisClient, rt.cfg.Auth, []byte(rt.cfg.Auth.LoginLimiterKey)),
			CookieSecure:       rt.cfg.Auth.CookieSecure,
			TrustedProxyHeader: rt.cfg.Auth.TrustedProxyHeader,
			TrustedProxyCIDRs:  proxyNetworks,
			Logger:             rt.log,
		}),
		ProviderController: llmcontroller.NewProviderController(services.ProviderService, services.ModelSyncService, maxBodyBytes),
		LLMModelController: llmcontroller.NewLLMModelController(services.LLMModelService, maxBodyBytes),
		RoleController:     llmcontroller.NewRoleController(services.RoleService, maxBodyBytes),
	}, nil
}
