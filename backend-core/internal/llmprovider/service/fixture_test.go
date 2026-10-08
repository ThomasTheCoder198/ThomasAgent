package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/client"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const secretKey = "sk-or-v1-supersecret"

func platformContext(ctx context.Context) context.Context {
	return tenant.WithID(ctx, tenant.PlatformID)
}

type fakeCatalog struct {
	models []model.RemoteModel
	err    error
	gotKey string
}

func (f *fakeCatalog) ListModels(_ context.Context, _ uuid.UUID, _ model.ProviderKind, _ string, apiKey string) ([]model.RemoteModel, error) {
	f.gotKey = apiKey
	return f.models, f.err
}

// serviceFixture embeds every service of the domain so a test can call any of them directly.
type serviceFixture struct {
	ProviderService
	LLMModelService
	ModelSyncService
	RoleService
	ResolveService
	pool    *pgxpool.Pool
	secrets *vault.Store
	catalog client.ModelCatalog
}

func newTestService(t *testing.T, catalog client.ModelCatalog) *serviceFixture {
	t.Helper()
	pool := pgtest.Start(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := vault.NewCipher("v1", base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	secrets := vault.NewStore(cipher)
	providerRepo := repository.NewProviderRepo()
	llmModelRepo := repository.NewLLMModelRepo(providerRepo)
	roleRepo := repository.NewRoleRepo()
	providerService := NewProviderService(pool, secrets, providerRepo, catalog)
	return &serviceFixture{
		ProviderService:  providerService,
		LLMModelService:  NewLLMModelService(pool, llmModelRepo),
		ModelSyncService: NewModelSyncService(pool, providerService, llmModelRepo, catalog, config.DefaultProviderMaxRemoteModels),
		RoleService:      NewRoleService(pool, llmModelRepo, roleRepo),
		ResolveService:   NewResolveService(pool, providerService, llmModelRepo, roleRepo),
		pool:             pool,
		secrets:          secrets,
		catalog:          catalog,
	}
}

func ptr[T any](v T) *T { return &v }

func code(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}

func seedProvider(t *testing.T, s *serviceFixture) model.Provider {
	t.Helper()
	provider, err := s.CreateProvider(platformContext(context.Background()), model.ProviderInput{Kind: model.KindOpenRouter, Name: "OR", APIKey: ptr(secretKey)})
	require.NoError(t, err)
	return provider
}
