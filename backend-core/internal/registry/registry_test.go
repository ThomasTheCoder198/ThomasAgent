package registry

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"testing"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/vault"
)

const secretKey = "sk-or-v1-supersecret"

type fakeCatalog struct {
	models []RemoteModel
	err    error
	gotKey string
}

func (f *fakeCatalog) ListModels(_ context.Context, _ ProviderKind, _ string, apiKey string) ([]RemoteModel, error) {
	f.gotKey = apiKey
	return f.models, f.err
}

func newTestService(t *testing.T, cat Catalog) *Service {
	t.Helper()
	pool := pgtest.Start(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := vault.NewCipher("v1", base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	return NewService(pool, vault.NewStore(c), cat)
}

func ptr[T any](v T) *T { return &v }

func code(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}

func TestCreateProviderStoresKeyWriteOnly(t *testing.T) {
	cat := &fakeCatalog{}
	s := newTestService(t, cat)
	ctx := context.Background()
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
	ctx := context.Background()
	_, err := s.CreateProvider(ctx, ProviderInput{Kind: "nope", Name: "x"})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenAICompatible, Name: "local"})
	require.Equal(t, errors.CodeValidationFailed, code(t, err), "openai_compatible needs a base URL")
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: " "})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
}

func TestProviderNameIsUnique(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := context.Background()
	_, err := s.CreateProvider(ctx, ProviderInput{Kind: KindOpenRouter, Name: "OR"})
	require.NoError(t, err)
	_, err = s.CreateProvider(ctx, ProviderInput{Kind: KindOpenAI, Name: "OR"})
	require.Equal(t, errors.CodeRegistryNameTaken, code(t, err))
}

func TestUpdateProviderKeySemantics(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := context.Background()
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
	require.Equal(t, errors.CodeRegistryProviderNotFound, code(t, s.DeleteProvider(context.Background(), uuid.New())))
}

func seedProvider(t *testing.T, s *Service) Provider {
	t.Helper()
	p, err := s.CreateProvider(context.Background(), ProviderInput{Kind: KindOpenRouter, Name: "OR", APIKey: ptr(secretKey)})
	require.NoError(t, err)
	return p
}

func TestSyncModelsUpsertsFromCatalog(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{
		{ModelRef: "anthropic/claude-sonnet-5", DisplayName: "Claude Sonnet 5", Capabilities: []Capability{CapChat, CapTools, CapVision}},
		{ModelRef: "openai/gpt-5-mini", DisplayName: "GPT-5 mini", Capabilities: []Capability{CapChat}},
	}}
	s := newTestService(t, cat)
	ctx := context.Background()
	p := seedProvider(t, s)

	res, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, SyncResult{Added: 2}, res)
	res, err = s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, SyncResult{Updated: 2}, res)

	vision := CapVision
	models, err := s.ListModels(ctx, nil, &vision)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "anthropic/claude-sonnet-5", models[0].ModelRef)
}

func TestSyncUnsupportedKind(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p, err := s.CreateProvider(context.Background(), ProviderInput{Kind: KindCohere, Name: "Cohere"})
	require.NoError(t, err)
	_, err = s.SyncModels(context.Background(), p.ID)
	require.Equal(t, errors.CodeRegistrySyncUnsupported, code(t, err))
}

func TestAssignRoleRejectsMissingCapability(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := context.Background()
	p := seedProvider(t, s)
	m, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/chat-only", DisplayName: "Chat only", Capabilities: []Capability{CapChat}})
	require.NoError(t, err)
	require.Equal(t, errors.CodeRegistryCapabilityMismatch, code(t, s.AssignRole(ctx, RoleChatDefault, m.ID)))
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))
}

func TestDeleteModelInUseIsConflict(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := context.Background()
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapChat}})
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))
	require.Equal(t, errors.CodeRegistryModelInUse, code(t, s.DeleteModel(ctx, m.ID)))
	require.Equal(t, errors.CodeRegistryModelInUse, code(t, s.DeleteProvider(ctx, p.ID)))
}

func TestResolveReturnsDecryptedKeyAndFallsBack(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := context.Background()
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapChat}})
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))

	got, err := s.Resolve(ctx, RoleChatFast)
	require.NoError(t, err)
	require.Equal(t, secretKey, got.APIKey)
	require.Equal(t, "x/fast", got.ModelRef)

	got, err = s.Resolve(ctx, RoleDecision)
	require.NoError(t, err)
	require.Equal(t, RoleChatFast, got.Role)
	require.Equal(t, RoleDecision, got.FallbackFrom)

	_, err = s.Resolve(ctx, RoleEmbedding)
	require.Equal(t, errors.CodeRegistryRoleNotAssigned, code(t, err))
}

func TestCreateModelValidatesCapabilities(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	_, err := s.CreateModel(context.Background(), ModelInput{ProviderID: p.ID, ModelRef: "x", DisplayName: "x", Capabilities: []Capability{"telepathy"}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
	_, err = s.CreateModel(context.Background(), ModelInput{ProviderID: p.ID, ModelRef: "e", DisplayName: "e", Capabilities: []Capability{CapEmbedding}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err), "embedding models need embeddingDims")
}

func catalogForTest(client *http.Client, maxBytes int64, attempts int, sleep func(context.Context, time.Duration) error) *HTTPCatalog {
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: attempts, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Second})
	if sleep != nil {
		policy.Sleep = sleep
	}
	breaker := retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 10, OpenTimeout: time.Second, HalfOpenMaxCalls: 1})
	return NewHTTPCatalogWithClient(policy, client, maxBytes, breaker, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCatalogMapsOpenRouterAndBoundsBody(t *testing.T) {
	payload := `{"data":[{"id":"sample/model","name":"Sample","context_length":8192,"architecture":{"input_modalities":["text","image"],"output_modalities":["text"]},"supported_parameters":["tools","reasoning"],"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "Bearer "+secretKey, r.Header.Get("Authorization"))
		_, err := io.WriteString(w, payload)
		require.NoError(t, err)
	}))
	defer srv.Close()
	cat := catalogForTest(srv.Client(), 4096, 1, nil)
	models, err := cat.ListModels(context.Background(), KindOpenRouter, srv.URL, secretKey)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.ElementsMatch(t, []Capability{CapChat, CapTools, CapVision, CapReasoning}, models[0].Capabilities)
	require.Equal(t, 1.0, *models[0].InputPricePerMTok)
	require.Equal(t, 2.0, *models[0].OutputPricePerMTok)
	cat = catalogForTest(srv.Client(), 10, 1, nil)
	_, err = cat.ListModels(context.Background(), KindOpenRouter, srv.URL, secretKey)
	require.Equal(t, errors.CodeRegistryProviderRejected, code(t, err))
	require.NotContains(t, err.Error(), secretKey)
}

func TestCatalogStatusRetryAfterAndMalformedBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		want     errors.Code
		attempts int
	}{
		{"unauthorized", 401, secretKey, errors.CodeRegistryProviderRejected, 1},
		{"rate limit", 429, secretKey, errors.CodeRateLimited, 2},
		{"server", 503, secretKey, errors.CodeProviderUnavailable, 2},
		{"malformed", 200, secretKey, errors.CodeRegistryProviderRejected, 1},
		{"missing data", 200, `{}`, errors.CodeRegistryProviderRejected, 1},
		{"trailing", 200, `{"data":[]}garbage`, errors.CodeRegistryProviderRejected, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			delays := []time.Duration{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(tc.status)
				_, err := io.WriteString(w, tc.body)
				require.NoError(t, err)
			}))
			defer srv.Close()
			cat := catalogForTest(srv.Client(), 4096, 2, func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil })
			_, err := cat.ListModels(context.Background(), KindOpenRouter, srv.URL, secretKey)
			require.Equal(t, tc.want, code(t, err))
			require.Equal(t, tc.attempts, calls)
			require.NotContains(t, err.Error(), secretKey)
			if tc.status == 429 {
				require.Equal(t, []time.Duration{2 * time.Second}, delays)
			}
		})
	}
}

func TestCatalogDefaultClientRejectsPrivateAndCredentialURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unsafe destination reached")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cat := NewHTTPCatalog(retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Second}), time.Second, retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 2, OpenTimeout: time.Second, HalfOpenMaxCalls: 1}))
	for _, endpoint := range []string{srv.URL, "https://username:" + secretKey + "@api.example.com/v1"} {
		_, err := cat.ListModels(context.Background(), KindOpenRouter, endpoint, secretKey)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretKey)
	}
}

func TestProviderResponseNeverContainsAPIKey(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secretKey)
	providers, err := s.ListProviders(context.Background())
	require.NoError(t, err)
	raw, err = json.Marshal(providers)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secretKey)
	var ciphertext []byte
	require.NoError(t, s.pool.QueryRow(context.Background(), "SELECT ciphertext FROM secrets").Scan(&ciphertext))
	require.NotContains(t, string(ciphertext), secretKey)
}

func TestProviderAuditRollback(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	_, err := s.pool.Exec(context.Background(), "CREATE FUNCTION fail_registry_audit() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_registry_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_registry_audit()")
	require.NoError(t, err)
	_, err = s.UpdateProvider(context.Background(), p.ID, ProviderInput{APIKey: ptr("replacement")})
	require.Error(t, err)
	_, key, err := s.providerWithKey(context.Background(), p.ID)
	require.NoError(t, err)
	require.Equal(t, secretKey, key)
}

func TestProviderRejectsUnsafeStoredURL(t *testing.T) {
	for _, endpoint := range []string{"https://username:" + secretKey + "@example.com/v1", "https://example.com/v1#fragment", "not-a-url"} {
		in := ProviderInput{Kind: KindOpenAICompatible, Name: "custom", BaseURL: endpoint}
		err := normalizeProvider(&in)
		require.Equal(t, errors.CodeValidationFailed, code(t, err))
		require.NotContains(t, err.Error(), secretKey)
	}
}

func TestModelRejectsInvalidNumericMetadata(t *testing.T) {
	for _, in := range []ModelInput{
		{ModelRef: "x", ContextWindow: ptr(-1)},
		{ModelRef: "x", EmbeddingDims: ptr(0)},
		{ModelRef: "x", InputPricePerMTok: ptr(-1.0)},
	} {
		require.Equal(t, errors.CodeValidationFailed, code(t, validateModel(&in)))
	}
}

func TestSyncRollsBackMalformedRemoteModel(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "valid", Capabilities: []Capability{CapChat}}, {ModelRef: " ", Capabilities: []Capability{CapChat}}}}
	s := newTestService(t, cat)
	p := seedProvider(t, s)
	_, err := s.SyncModels(context.Background(), p.ID)
	require.Error(t, err)
	models, err := s.ListModels(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestResolveDisabledProviderIsUnavailable(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	m, err := s.CreateModel(context.Background(), ModelInput{ProviderID: p.ID, ModelRef: "x", Capabilities: []Capability{CapChat}})
	require.NoError(t, err)
	require.NoError(t, s.AssignRole(context.Background(), RoleChatFast, m.ID))
	_, err = s.UpdateProvider(context.Background(), p.ID, ProviderInput{Enabled: ptr(false)})
	require.NoError(t, err)
	_, err = s.Resolve(context.Background(), RoleChatFast)
	require.Equal(t, errors.CodeProviderUnavailable, code(t, err))
}

func TestCatalogChunkedOversizeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, err := io.WriteString(w, `{"data":[],"padding":"`+strings.Repeat("x", 128)+`"}`)
		require.NoError(t, err)
	}))
	defer srv.Close()
	cat := catalogForTest(srv.Client(), 64, 1, nil)
	_, err := cat.ListModels(context.Background(), KindOpenRouter, srv.URL, secretKey)
	require.Equal(t, errors.CodeRegistryProviderRejected, code(t, err))
}

func TestCatalogUnknownPricesRemainNullable(t *testing.T) {
	for _, value := range []string{"-1", "NaN", "Inf", "unknown"} {
		require.Nil(t, perMillion(value))
	}
}
