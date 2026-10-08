package llm

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
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
	models []RemoteModel
	err    error
	gotKey string
}

func (f *fakeCatalog) ListModels(_ context.Context, _ uuid.UUID, _ ProviderKind, _ string, apiKey string) ([]RemoteModel, error) {
	f.gotKey = apiKey
	return f.models, f.err
}

type serviceFixture struct {
	*Service
	pool    *pgxpool.Pool
	secrets *vault.Store
	catalog ModelCatalog
}

func newTestService(t *testing.T, cat ModelCatalog) *serviceFixture {
	t.Helper()
	pool := pgtest.Start(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := vault.NewCipher("v1", base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	secrets := vault.NewStore(c)
	return &serviceFixture{
		Service: NewService(pool, secrets, cat, config.DefaultProviderMaxRemoteModels),
		pool:    pool, secrets: secrets, catalog: cat,
	}
}

func ptr[T any](v T) *T { return &v }

func code(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}

func seedProvider(t *testing.T, s *serviceFixture) Provider {
	t.Helper()
	p, err := s.CreateProvider(platformContext(context.Background()), ProviderInput{Kind: KindOpenRouter, Name: "OR", APIKey: ptr(secretKey)})
	require.NoError(t, err)
	return p
}
