package llm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestCreateProviderStoresKeyWriteOnly(t *testing.T) {
	cat := &fakeCatalog{}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	p, err := s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: "OpenRouter", APIKey: ptr(secretKey)})
	require.NoError(t, err)
	require.True(t, p.HasAPIKey)
	require.Equal(t, "https://openrouter.ai/api/v1", p.BaseURL)

	_, err = s.TestProvider(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, secretKey, cat.gotKey, "the decrypted key reaches the provider call")
}

func TestCreateProviderValidates(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	_, err := s.CreateProvider(ctx, ProviderInput{Kind: "nope", Name: "x"})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenAICompatible, Name: "local"})
	require.Equal(t, errors.CodeValidationFailed, code(t, err), "openai_compatible needs a base URL")
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: " "})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
}

func TestProviderNameIsUnique(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	_, err := s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: "OR"})
	require.NoError(t, err)
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenAI, Name: "OR"})
	require.Equal(t, errors.CodeLLMNameTaken, code(t, err))
}

func TestUpdateProviderKeySemantics(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p, _ := s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: "OR", APIKey: ptr(secretKey)})
	p, err := s.UpdateProvider(ctx, p.ID, ProviderInput{Name: "OR renamed"})
	require.NoError(t, err)
	require.True(t, p.HasAPIKey, "nil APIKey leaves the key unchanged")
	p, err = s.UpdateProvider(ctx, p.ID, ProviderInput{APIKey: ptr("")})
	require.NoError(t, err)
	require.False(t, p.HasAPIKey, "empty APIKey removes the key")
}

func TestDeleteUnknownProvider(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	require.Equal(t, errors.CodeLLMProviderNotFound, code(t, s.DeleteProvider(platformContext(context.Background()), uuid.New())))
}

func TestUpdateProviderBaseURLClearsStoredKey(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	updated, err := s.UpdateProvider(platformContext(context.Background()), p.ID, ProviderInput{BaseURL: "https://new.example/v1"})
	require.NoError(t, err)
	require.False(t, updated.HasAPIKey)
}

func TestProviderResponseNeverContainsAPIKey(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	raw, err := json.Marshal(newProviderResponse(p))
	require.NoError(t, err)
	require.NotContains(t, string(raw), secretKey)
	providers, err := s.ListProviders(platformContext(context.Background()))
	require.NoError(t, err)
	raw, err = json.Marshal(newProviderResponses(providers))
	require.NoError(t, err)
	require.NotContains(t, string(raw), secretKey)
	var ciphertext []byte
	require.NoError(t, s.pool.QueryRow(platformContext(context.Background()), "SELECT ciphertext FROM secrets").Scan(&ciphertext))
	require.NotContains(t, string(ciphertext), secretKey)
}

func TestProviderAuditRollback(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	_, err := s.pool.Exec(platformContext(context.Background()), "CREATE FUNCTION fail_registry_audit() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_registry_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_registry_audit()")
	require.NoError(t, err)
	_, err = s.UpdateProvider(platformContext(context.Background()), p.ID, ProviderInput{APIKey: ptr("replacement")})
	require.Error(t, err)
	stored, err := NewRepository().GetProvider(platformContext(context.Background()), s.pool, p.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.SecretID)
	key, err := s.secrets.Get(platformContext(context.Background()), s.pool, *stored.SecretID)
	require.NoError(t, err)
	require.Equal(t, secretKey, key)
}

func TestProviderRejectsUnsafeStoredURL(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	for _, endpoint := range []string{"https://username:" + secretKey + "@example.com/v1", "https://example.com/v1#fragment", "not-a-url"} {
		in := ProviderInput{Kind: KindOpenAICompatible, Name: "custom", BaseURL: endpoint}
		_, err := s.CreateProvider(platformContext(t.Context()), in)
		require.Equal(t, errors.CodeValidationFailed, code(t, err))
		require.NotContains(t, err.Error(), secretKey)
	}
}
