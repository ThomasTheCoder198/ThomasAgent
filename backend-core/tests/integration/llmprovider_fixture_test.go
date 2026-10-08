package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	llmservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const (
	providerSecretKey = "sk-or-v1-supersecret"
	serviceToken      = "svc-token"
	testMaxBodyBytes  = 64 << 10
)

func platformContext(ctx context.Context) context.Context {
	return tenant.WithID(ctx, tenant.PlatformID)
}

type fakeCatalog struct {
	models []llmmodel.RemoteModel
	err    error
}

func (f *fakeCatalog) ListModels(context.Context, uuid.UUID, llmmodel.ProviderKind, string, string) ([]llmmodel.RemoteModel, error) {
	return f.models, f.err
}

// llmServices are the services behind the public provider routes; zero values are fine for
// requests the controller rejects before calling a service.
type llmServices struct {
	providers llmservice.ProviderService
	models    llmservice.LLMModelService
	modelSync llmservice.ModelSyncService
	roles     llmservice.RoleService
	resolve   llmservice.ResolveService
}

type llmFixture struct {
	llmServices
	pool    *pgxpool.Pool
	catalog *fakeCatalog
}

func newLLMFixture(t *testing.T) *llmFixture {
	t.Helper()
	pool := pgtest.Start(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := vault.NewCipher("v1", base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	catalog := &fakeCatalog{}
	providerRepo := llmrepository.NewProviderRepo()
	llmModelRepo := llmrepository.NewLLMModelRepo(providerRepo)
	roleRepo := llmrepository.NewRoleRepo()
	providerService := llmservice.NewProviderService(pool, vault.NewStore(cipher), providerRepo, catalog)
	return &llmFixture{
		llmServices: llmServices{
			providers: providerService,
			models:    llmservice.NewLLMModelService(pool, llmModelRepo),
			modelSync: llmservice.NewModelSyncService(pool, providerService, llmModelRepo, catalog, config.DefaultProviderMaxRemoteModels),
			roles:     llmservice.NewRoleService(pool, llmModelRepo, roleRepo),
			resolve:   llmservice.NewResolveService(pool, providerService, llmModelRepo, roleRepo),
		},
		pool:    pool,
		catalog: catalog,
	}
}

// mountProvidersForTest registers the public provider routes exactly as cmd/core/routes.go does.
func mountProvidersForTest(routes gin.IRoutes, services llmServices, maxBodyBytes int64) {
	providers := llmcontroller.NewProviderController(services.providers, services.modelSync, maxBodyBytes)
	models := llmcontroller.NewLLMModelController(services.models, maxBodyBytes)
	roles := llmcontroller.NewRoleController(services.roles, maxBodyBytes)
	routes.GET("/api/v1/providers", providers.HandleListProviders)
	routes.POST("/api/v1/providers", providers.HandleCreateProvider)
	routes.PATCH("/api/v1/providers/:providerId", providers.HandleUpdateProvider)
	routes.DELETE("/api/v1/providers/:providerId", providers.HandleDeleteProvider)
	routes.POST("/api/v1/providers/:providerId/test", providers.HandleTestProvider)
	routes.POST("/api/v1/providers/:providerId/sync", providers.HandleSyncProvider)
	routes.GET("/api/v1/models", models.HandleListModels)
	routes.POST("/api/v1/models", models.HandleCreateModel)
	routes.DELETE("/api/v1/models/:modelId", models.HandleDeleteModel)
	routes.GET("/api/v1/model-roles", roles.HandleListRoles)
	routes.PUT("/api/v1/model-roles/:role", roles.HandleAssignRole)
}

// newProviderRouter serves the provider routes in the platform tenant without a session, plus the internal resolve route.
func newProviderRouter(t *testing.T, logs *bytes.Buffer) (*gin.Engine, *llmFixture) {
	t.Helper()
	fixture := newLLMFixture(t)
	logger, err := logging.NewLogger(logs, "debug", "test", "test", nil)
	require.NoError(t, err)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	router.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(platformContext(ctx.Request.Context()))
		ctx.Next()
	})
	mountProvidersForTest(router, fixture.llmServices, testMaxBodyBytes)
	router.GET("/internal/models/resolve", authmiddleware.RequireServiceToken(serviceToken), llmcontroller.NewResolveController(fixture.resolve).HandleResolve)
	return router, fixture
}

func seedProvider(t *testing.T, fixture *llmFixture) llmmodel.Provider {
	t.Helper()
	apiKey := providerSecretKey
	provider, err := fixture.providers.CreateProvider(platformContext(context.Background()), llmmodel.ProviderInput{Kind: llmmodel.KindOpenRouter, Name: "OR", APIKey: &apiKey})
	require.NoError(t, err)
	return provider
}

func send(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
