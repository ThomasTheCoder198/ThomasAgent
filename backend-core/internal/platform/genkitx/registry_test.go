package genkitx_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx/genkitxtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const testCacheSize = 4

func resolved(kind llmmodel.ProviderKind, base, key, keyVersion string) llmmodel.ResolvedModel {
	return llmmodel.ResolvedModel{
		ProviderID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), ProviderKind: kind, BaseURL: base,
		APIKey: key, KeyVersion: keyVersion, ModelRef: "test/model",
		Capabilities: []llmmodel.Capability{llmmodel.CapabilityChat, llmmodel.CapabilityTools},
	}
}

func reply(t *testing.T, ctx context.Context, m genkitx.ChatModel) string {
	t.Helper()
	opts := append(m.Options(16), ai.WithMessages(ai.NewUserTextMessage("hi")))
	text, err := genkit.GenerateText(ctx, m.G, opts...)
	require.NoError(t, err)
	return text
}

func TestRegistry_OpenRouterSharesOneInstanceAndSendsEachRequestKey(t *testing.T) {
	ctx := context.Background()
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "a"}, genkitxtest.Turn{Text: "b"})
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: testCacheSize})

	first, err := registry.Chat(ctx, resolved(llmmodel.KindOpenRouter, server.URL+"/api/v1", "sk-one", "v1"))
	require.NoError(t, err)
	second, err := registry.Chat(ctx, resolved(llmmodel.KindOpenRouter, server.URL+"/api/v1", "sk-two", "v2"))
	require.NoError(t, err)

	require.Same(t, first.G, second.G)
	require.Equal(t, "openrouter/test/model", first.Name)
	require.Equal(t, "a", reply(t, ctx, first))
	require.Equal(t, "b", reply(t, ctx, second))
	requests := server.Requests()
	require.Equal(t, "Bearer sk-one", requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer sk-two", requests[1].Header.Get("Authorization"))
}

func TestRegistry_CompatInstanceFollowsKeyVersion(t *testing.T) {
	ctx := context.Background()
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "a"}, genkitxtest.Turn{Text: "b"})
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: testCacheSize})

	old, err := registry.Chat(ctx, resolved(llmmodel.KindOpenAICompatible, server.URL, "sk-old", "v1"))
	require.NoError(t, err)
	rotated, err := registry.Chat(ctx, resolved(llmmodel.KindOpenAICompatible, server.URL, "sk-new", "v2"))
	require.NoError(t, err)

	require.NotSame(t, old.G, rotated.G)
	reply(t, ctx, rotated)
	require.Equal(t, "Bearer sk-new", server.Requests()[0].Header.Get("Authorization"))
}

func TestRegistry_LeastRecentlyUsedInstanceIsEvicted(t *testing.T) {
	ctx := context.Background()
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1})
	a, err := registry.Chat(ctx, resolved(llmmodel.KindOpenAICompatible, "http://a.invalid", "k", "v1"))
	require.NoError(t, err)
	_, err = registry.Chat(ctx, resolved(llmmodel.KindOpenAICompatible, "http://b.invalid", "k", "v1"))
	require.NoError(t, err)
	again, err := registry.Chat(ctx, resolved(llmmodel.KindOpenAICompatible, "http://a.invalid", "k", "v1"))
	require.NoError(t, err)
	require.NotSame(t, a.G, again.G)
}

func TestRegistry_RejectsKindsWithoutAVerifiedAdapter(t *testing.T) {
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: testCacheSize})
	for _, kind := range []llmmodel.ProviderKind{llmmodel.KindAnthropic, llmmodel.KindGoogle, llmmodel.KindAzure} {
		_, err := registry.Chat(context.Background(), resolved(kind, "", "k", "v1"))
		require.ErrorIs(t, err, errors.ErrLLMProviderRejected, kind)
	}
	_, err := registry.Decision(context.Background(), resolved(llmmodel.KindOpenAI, "", "k", "v1"))
	require.ErrorIs(t, err, errors.ErrLLMProviderRejected)
	_, err = registry.Decision(context.Background(), resolved(llmmodel.KindOpenRouter, "", "", ""))
	require.ErrorIs(t, err, errors.ErrLLMProviderRejected, "a decision model needs a stored key")
	_, err = registry.Chat(context.Background(), resolved(llmmodel.KindOpenRouter, "", "", ""))
	require.ErrorIs(t, err, errors.ErrLLMProviderRejected,
		"an empty per-request key would fall back to the plugin placeholder; reject before any request")
}

func TestRegistry_DecisionInstancePerKeyVersion(t *testing.T) {
	ctx := context.Background()
	server := genkitxtest.NewDecisionsServer(t, map[string]any{"reachable": map[string]any{"type": "noul", "noul": 0.9}})
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: testCacheSize})
	m := resolved(llmmodel.KindOpenRouter, server.URL+"/api/v1", "sk-dec", "v1")
	m.ModelRef = "typesafe/jev-1.14"

	first, err := registry.Decision(ctx, m)
	require.NoError(t, err)
	same, err := registry.Decision(ctx, m)
	require.NoError(t, err)
	m.KeyVersion = "v2"
	rotated, err := registry.Decision(ctx, m)
	require.NoError(t, err)

	require.Same(t, first.G, same.G)
	require.NotSame(t, first.G, rotated.G)
	require.Equal(t, "openrouter-decisions/typesafe/jev-1.14", first.Name)
}

// Review Focus #1: the key is only ever the bearer token, and no internal id reaches the provider.
func TestRegistry_KeyNeverInRequestBodyOrHeaders(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), "req_registry-key-check")
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "ok"})
	registry := genkitx.NewModelRegistry(genkitx.Config{CacheSize: testCacheSize})
	m, err := registry.Chat(ctx, resolved(llmmodel.KindOpenRouter, server.URL+"/api/v1", "sk-secret-value", "v1"))
	require.NoError(t, err)

	reply(t, ctx, m)

	request := server.Requests()[0]
	body, err := json.Marshal(request.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "sk-secret-value")
	require.Empty(t, request.Header.Get(requestid.Header))
	for name, values := range request.Header {
		if name == "Authorization" {
			continue
		}
		for _, value := range values {
			require.NotContains(t, value, "sk-secret-value", name)
		}
	}
}
