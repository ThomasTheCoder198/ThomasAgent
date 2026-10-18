package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	agentcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/controller"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	agentservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/service"
	appcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/controller"
	appservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/service"
	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	convcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/controller"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	llmclient "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/client"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	llmservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	orgcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/controller"
	orgrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	orgservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
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
	CallLog      *calllog.Logger
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
	var llmRecorder calllog.LLMRecorder
	if rt.metrics != nil {
		llmRecorder = rt.metrics
	}
	callLog := calllog.New(rt.log, llmRecorder, rt.cfg.LLMLog)
	signer := apptoken.NewAppTokenSigner(signingKey, rt.cfg.App, time.Now, uuid.NewString)
	ragxCaller, err := ragx.NewRagxCaller(rt.cfg.Ragx, signer, recorder, rt.log, callLog)
	if err != nil {
		return ClientContainer{}, fmt.Errorf("ragx client: %w", err)
	}
	return ClientContainer{ModelCatalog: modelCatalog, Ragx: ragxCaller, CallLog: callLog}, nil
}

type RepositoryContainer struct {
	AgentRepo        agentrepository.AgentRepo
	ConversationRepo convrepository.ConversationRepo
	MessageRepo      convrepository.MessageRepo
	RunRepo          convrepository.RunRepo
	UserRepo         authrepository.UserRepo
	SessionRepo      authrepository.SessionRepo
	ProviderRepo     llmrepository.ProviderRepo
	LLMModelRepo     llmrepository.LLMModelRepo
	RoleRepo         llmrepository.RoleRepo

	OrganizationRepo orgrepository.OrganizationRepo
}

func (rt *coreRuntime) initRepositoryContainer(pool *pgxpool.Pool) RepositoryContainer {
	providerRepo := llmrepository.NewProviderRepo()
	return RepositoryContainer{
		AgentRepo:        agentrepository.NewAgentRepo(),
		ConversationRepo: convrepository.NewConversationRepo(),
		MessageRepo:      convrepository.NewMessageRepo(),
		RunRepo:          convrepository.NewRunRepo(),
		UserRepo:         authrepository.NewUserRepo(pool),
		SessionRepo:      authrepository.NewSessionRepo(pool, rt.cfg.Auth.SessionTouchInterval),
		ProviderRepo:     providerRepo,
		LLMModelRepo:     llmrepository.NewLLMModelRepo(providerRepo),
		RoleRepo:         llmrepository.NewRoleRepo(),

		OrganizationRepo: orgrepository.NewOrganizationRepo(),
	}
}

type ServiceContainer struct {
	AgentService        agentservice.AgentService
	ConversationService convservice.ConversationService
	// ChatStore is the persistence port of the M2b run loop.
	ChatStore        convservice.ChatStore
	AuthService      authservice.AuthService
	ProviderService  llmservice.ProviderService
	LLMModelService  llmservice.LLMModelService
	ModelSyncService llmservice.ModelSyncService
	RoleService      llmservice.RoleService
	ResolveService   llmservice.ResolveService
	OrgRoleService   llmservice.OrgRoleService

	OrganizationService orgservice.OrganizationService
	AppProxyService     appservice.AppProxyService
}

// initServiceContainer also bootstraps the owner account, so startup fails before serving if it cannot.
func (rt *coreRuntime) initServiceContainer(ctx context.Context, pool *pgxpool.Pool, cipher *vault.Cipher, clients ClientContainer, repositories RepositoryContainer) (ServiceContainer, error) {
	authService := authservice.NewAuthService(repositories.UserRepo, repositories.SessionRepo, pool, rt.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return ServiceContainer{}, err
	}
	providerService := llmservice.NewProviderService(pool, vault.NewStore(cipher), repositories.ProviderRepo, clients.ModelCatalog)
	// CORE_RAGX_PROVISION_ATTEMPTS, not cfg.Retry.MaxAttempts: the config checks this budget against the request timeout.
	provisionPolicy := retry.NewPolicy(config.RetryConfig{
		MaxAttempts: rt.cfg.Ragx.ProvisionAttempts, BaseDelay: rt.cfg.Retry.BaseDelay, MaxDelay: rt.cfg.Retry.MaxDelay,
	})
	resolveService := llmservice.NewResolveService(pool, providerService, repositories.LLMModelRepo, repositories.RoleRepo)
	organizationService := orgservice.NewOrganizationService(pool, repositories.OrganizationRepo, clients.Ragx, provisionPolicy, uuid.NewString)
	return ServiceContainer{
		AgentService: agentservice.NewAgentService(pool, repositories.AgentRepo, clients.Ragx, resolveService, rt.log),
		ConversationService: convservice.NewConversationService(pool, repositories.ConversationRepo, repositories.MessageRepo,
			repositories.AgentRepo, clients.Ragx, rt.cfg.Conversation, rt.log),
		ChatStore:        convservice.NewChatStore(pool, repositories.ConversationRepo, repositories.MessageRepo, repositories.RunRepo, repositories.AgentRepo),
		AuthService:      authService,
		ProviderService:  providerService,
		LLMModelService:  llmservice.NewLLMModelService(pool, repositories.LLMModelRepo, repositories.RoleRepo),
		ModelSyncService: llmservice.NewModelSyncService(pool, providerService, repositories.LLMModelRepo, clients.ModelCatalog, rt.cfg.ProviderMaxRemoteModels),
		RoleService:      llmservice.NewRoleService(pool, repositories.LLMModelRepo, repositories.RoleRepo),
		ResolveService:   resolveService,
		OrgRoleService:   llmservice.NewOrgRoleService(pool, repositories.LLMModelRepo, repositories.RoleRepo),

		OrganizationService: organizationService,
		AppProxyService:     appservice.NewAppProxyService(clients.Ragx),
	}, nil
}

type ControllerContainer struct {
	AgentController        *agentcontroller.AgentController
	ConversationController *convcontroller.ConversationController
	AuthController         *authcontroller.AuthController
	ProviderController     *llmcontroller.ProviderController
	LLMModelController     *llmcontroller.LLMModelController
	RoleController         *llmcontroller.RoleController
	OrgRoleController      *llmcontroller.OrgRoleController

	OrganizationController *orgcontroller.OrganizationController
	AppProxyController     *appcontroller.AppProxyController
}

func (rt *coreRuntime) initControllerContainer(services ServiceContainer, redisClient *redis.Client) (ControllerContainer, error) {
	proxyNetworks, err := parseTrustedProxyCIDRs(rt.cfg.Auth.TrustedProxyCIDRs)
	if err != nil {
		return ControllerContainer{}, err
	}
	maxBodyBytes := rt.cfg.HTTP.MaxBodyBytes
	return ControllerContainer{
		AgentController:        agentcontroller.NewAgentController(services.AgentService, maxBodyBytes),
		ConversationController: convcontroller.NewConversationController(services.ConversationService, rt.cfg.Conversation, maxBodyBytes),
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
		OrgRoleController:  llmcontroller.NewOrgRoleController(services.OrgRoleService, maxBodyBytes),

		OrganizationController: orgcontroller.NewOrganizationController(services.OrganizationService, maxBodyBytes),
		AppProxyController:     appcontroller.NewAppProxyController(services.AppProxyService, maxBodyBytes),
	}, nil
}
