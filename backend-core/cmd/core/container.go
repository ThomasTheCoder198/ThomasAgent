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
	chatservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/service"
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
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
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
	ModelCatalog  *llmclient.HTTPModelCatalog
	Ragx          *ragx.Caller
	CallLog       *calllog.Logger
	ModelRegistry *genkitx.Registry
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
	var llmRecorder calllog.LLMRecorder
	if rt.metrics != nil { // a nil *Metrics inside an interface would panic on the first call
		recorder, llmRecorder = rt.metrics, rt.metrics
	}
	llmHTTP := *httpClient
	llmHTTP.Timeout = 0
	llmHTTP.Transport = outbound.Instrument(httpClient.Transport, outbound.TargetLLM, recorder, false)
	modelRegistry := genkitx.NewModelRegistry(genkitx.Config{
		CacheSize: rt.cfg.LLM.GenkitCacheSize, HTTPClient: &llmHTTP, MaxRetries: rt.cfg.Retry.MaxAttempts - 1,
	})
	callLog := calllog.New(rt.log, llmRecorder, rt.cfg.LLMLog)
	signer := apptoken.NewAppTokenSigner(signingKey, rt.cfg.App, time.Now, uuid.NewString)
	ragxCaller, err := ragx.NewRagxCaller(rt.cfg.Ragx, signer, recorder, rt.log, callLog)
	if err != nil {
		return ClientContainer{}, fmt.Errorf("ragx client: %w", err)
	}
	return ClientContainer{ModelCatalog: modelCatalog, Ragx: ragxCaller, CallLog: callLog, ModelRegistry: modelRegistry}, nil
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
	RetentionService    convservice.RetentionService
	// ChatStore is the persistence port of the chat run; RunService is its loop and pre-run checks (the chat endpoint,
	// 2b-5, calls Validate and CheckModel before BeginRun; `core worker` runs Execute).
	ChatStore         convservice.ChatStore
	RunService        chatservice.RunService
	AuthService       authservice.AuthService
	ProviderService   llmservice.ProviderService
	LLMModelService   llmservice.LLMModelService
	ModelSyncService  llmservice.ModelSyncService
	RoleService       llmservice.RoleService
	ResolveService    llmservice.ResolveService
	ModelProbeService llmservice.ModelProbeService
	OrgRoleService    llmservice.OrgRoleService

	OrganizationService orgservice.OrganizationService
	AppProxyService     appservice.AppProxyService
}

// newModelServices builds the provider vault service and the role resolver; `core serve` and `core worker` share them.
func newModelServices(pool *pgxpool.Pool, cipher *vault.Cipher, clients ClientContainer, repositories RepositoryContainer) (llmservice.ProviderService, llmservice.ResolveService) {
	providerService := llmservice.NewProviderService(pool, vault.NewStore(cipher), repositories.ProviderRepo, clients.ModelCatalog)
	return providerService, llmservice.NewResolveService(pool, providerService, repositories.LLMModelRepo, repositories.RoleRepo)
}

// newRunService is the one place the run loop is assembled, so the API's pre-run checks and the worker's executor use
// the same model resolution and ragx caller.
func (rt *coreRuntime) newRunService(store convservice.ChatStore, agents agentservice.AgentService, resolve llmservice.ResolveService, clients ClientContainer) (chatservice.RunService, error) {
	return chatservice.NewRunService(chatservice.RunDeps{
		Store: store, Agents: agents, Models: resolve, Registry: clients.ModelRegistry, Tools: clients.Ragx, Calls: clients.CallLog,
		Config: rt.cfg.Chat, ToolTimeout: rt.cfg.Ragx.ToolTimeout, Log: rt.log,
	})
}

// initWorkerRunService builds the run loop for `core worker` without the API's owner bootstrap.
func (rt *coreRuntime) initWorkerRunService(pool *pgxpool.Pool, cipher *vault.Cipher, signingKey ed25519.PrivateKey) (chatservice.RunService, error) {
	clients, err := rt.initClientContainer(signingKey)
	if err != nil {
		return nil, err
	}
	repositories := rt.initRepositoryContainer(pool)
	_, resolveService := newModelServices(pool, cipher, clients, repositories)
	agentService := agentservice.NewAgentService(pool, repositories.AgentRepo, clients.Ragx, resolveService, rt.log)
	chatStore := convservice.NewChatStore(pool, repositories.ConversationRepo, repositories.MessageRepo, repositories.RunRepo, repositories.AgentRepo, rt.cfg.Chat.FinishTimeout)
	return rt.newRunService(chatStore, agentService, resolveService, clients)
}

// initServiceContainer also bootstraps the owner account, so startup fails before serving if it cannot.
func (rt *coreRuntime) initServiceContainer(ctx context.Context, pool *pgxpool.Pool, cipher *vault.Cipher, clients ClientContainer, repositories RepositoryContainer) (ServiceContainer, error) {
	authService := authservice.NewAuthService(repositories.UserRepo, repositories.SessionRepo, pool, rt.cfg.Auth, time.Now)
	if err := authService.EnsureOwner(tenant.WithID(ctx, tenant.PlatformID)); err != nil {
		return ServiceContainer{}, err
	}
	providerService, resolveService := newModelServices(pool, cipher, clients, repositories)
	// CORE_RAGX_PROVISION_ATTEMPTS, not cfg.Retry.MaxAttempts: the config checks this budget against the request timeout.
	provisionPolicy := retry.NewPolicy(config.RetryConfig{
		MaxAttempts: rt.cfg.Ragx.ProvisionAttempts, BaseDelay: rt.cfg.Retry.BaseDelay, MaxDelay: rt.cfg.Retry.MaxDelay,
	})
	organizationService := orgservice.NewOrganizationService(pool, repositories.OrganizationRepo, clients.Ragx, provisionPolicy, uuid.NewString)
	agentService := agentservice.NewAgentService(pool, repositories.AgentRepo, clients.Ragx, resolveService, rt.log)
	chatStore := convservice.NewChatStore(pool, repositories.ConversationRepo, repositories.MessageRepo, repositories.RunRepo, repositories.AgentRepo, rt.cfg.Chat.FinishTimeout)
	runService, err := rt.newRunService(chatStore, agentService, resolveService, clients)
	if err != nil {
		return ServiceContainer{}, err
	}
	return ServiceContainer{
		AgentService: agentService,
		ConversationService: convservice.NewConversationService(pool, repositories.ConversationRepo, repositories.MessageRepo, repositories.RunRepo,
			repositories.AgentRepo, clients.Ragx, rt.cfg.Conversation, rt.log),
		RetentionService:  convservice.NewRetentionService(pool, repositories.ConversationRepo, rt.cfg.Conversation, time.Now),
		ChatStore:         chatStore,
		RunService:        runService,
		AuthService:       authService,
		ProviderService:   providerService,
		LLMModelService:   llmservice.NewLLMModelService(pool, repositories.LLMModelRepo, repositories.RoleRepo),
		ModelSyncService:  llmservice.NewModelSyncService(pool, providerService, repositories.LLMModelRepo, clients.ModelCatalog, rt.cfg.ProviderMaxRemoteModels),
		RoleService:       llmservice.NewRoleService(pool, repositories.LLMModelRepo, repositories.RoleRepo),
		ResolveService:    resolveService,
		ModelProbeService: llmservice.NewModelProbeService(resolveService, clients.ModelRegistry, clients.CallLog, rt.cfg.LLM.ProbeTimeout),
		OrgRoleService:    llmservice.NewOrgRoleService(pool, repositories.LLMModelRepo, repositories.RoleRepo),

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
	ModelProbeController   *llmcontroller.ModelProbeController
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
		ProviderController:   llmcontroller.NewProviderController(services.ProviderService, services.ModelSyncService, maxBodyBytes),
		LLMModelController:   llmcontroller.NewLLMModelController(services.LLMModelService, maxBodyBytes),
		ModelProbeController: llmcontroller.NewModelProbeController(services.ModelProbeService),
		RoleController:       llmcontroller.NewRoleController(services.RoleService, maxBodyBytes),
		OrgRoleController:    llmcontroller.NewOrgRoleController(services.OrgRoleService, maxBodyBytes),

		OrganizationController: orgcontroller.NewOrganizationController(services.OrganizationService, maxBodyBytes),
		AppProxyController:     appcontroller.NewAppProxyController(services.AppProxyService, maxBodyBytes),
	}, nil
}
