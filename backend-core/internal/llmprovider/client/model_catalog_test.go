package client

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

func catalogForTest(client *http.Client, maxBytes int64, attempts int, sleep func(context.Context, time.Duration) error) *HTTPModelCatalog {
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: attempts, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Second})
	if sleep != nil {
		policy.Sleep = sleep
	}
	breaker := retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 10, OpenTimeout: time.Second, HalfOpenMaxCalls: 1})
	return NewHTTPModelCatalogWithClient(policy, client, maxBytes, breaker, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	models, err := cat.ListModels(platformContext(context.Background()), uuid.New(), model.KindOpenRouter, srv.URL, secretKey)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.ElementsMatch(t, []model.Capability{model.CapabilityChat, model.CapabilityTools, model.CapabilityVision, model.CapabilityReasoning}, models[0].Capabilities)
	require.Equal(t, 1.0, *models[0].InputPricePerMTok)
	require.Equal(t, 2.0, *models[0].OutputPricePerMTok)
	cat = catalogForTest(srv.Client(), 10, 1, nil)
	_, err = cat.ListModels(platformContext(context.Background()), uuid.New(), model.KindOpenRouter, srv.URL, secretKey)
	require.Equal(t, errors.CodeLLMProviderRejected, code(t, err))
	require.NotContains(t, err.Error(), secretKey)
}

func TestCatalogKeepsValidEntriesWhenOneRemoteModelIsMalformed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"valid"},{"id":123},{"id":"missing-context"},{"id":"negative-context","context_length":-1}]}`)
	}))
	defer server.Close()
	cat := catalogForTest(server.Client(), 4096, 1, nil)
	models, err := cat.ListModels(platformContext(t.Context()), uuid.New(), model.KindOpenRouter, server.URL, "")
	require.NoError(t, err)
	require.Len(t, models, 4)
	require.Equal(t, "valid", models[0].ModelRef)
	require.Empty(t, models[1].ModelRef, "invalid entries must reach sync's skip counter")
	require.Nil(t, models[2].ContextWindow)
	require.Nil(t, models[3].ContextWindow)
}

func TestCatalogMaintainsIndependentProviderBreakers(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer failing.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"data":[{"id":"healthy"}]}`) }))
	defer healthy.Close()
	cat := catalogForTest(http.DefaultClient, 1024, 1, nil)
	cat.Breaker = retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 1, OpenTimeout: time.Hour, HalfOpenMaxCalls: 1})
	oneID, anotherID := uuid.New(), uuid.New()
	_, err := cat.ListModels(platformContext(t.Context()), oneID, model.KindOpenRouter, failing.URL, "")
	require.Error(t, err)
	// The first provider stays open even if its next URL responds successfully.
	_, err = cat.ListModels(platformContext(t.Context()), oneID, model.KindOpenRouter, healthy.URL, "")
	require.Error(t, err)
	models, err := cat.ListModels(platformContext(t.Context()), anotherID, model.KindOpenRouter, healthy.URL, "")
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "healthy", models[0].ModelRef)
}

func TestCatalogBoundsBreakerCacheAndKeepsOpenProvidersProtected(t *testing.T) {
	var failingCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fail") {
			failingCalls.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"healthy"}]}`)
	}))
	defer server.Close()
	cat := catalogForTest(server.Client(), 1024, 1, nil)
	cat.Breaker = retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 1, OpenTimeout: time.Hour, HalfOpenMaxCalls: 1})
	failedProvider := uuid.New()
	_, err := cat.ListModels(t.Context(), failedProvider, model.KindOpenRouter, server.URL+"/fail", "")
	require.Error(t, err)
	for range 300 {
		_, err = cat.ListModels(t.Context(), uuid.New(), model.KindOpenRouter, server.URL, "")
		require.NoError(t, err)
	}
	_, err = cat.ListModels(t.Context(), failedProvider, model.KindOpenRouter, server.URL+"/fail", "")
	require.Error(t, err)
	require.Equal(t, int32(1), failingCalls.Load(), "cache churn cannot reset an open circuit")
	require.LessOrEqual(t, len(cat.breakers), 256)
}

func TestCatalogCachePressureDoesNotEvictActiveProvider(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/blocked") {
			once.Do(func() { close(entered) })
			<-release
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"healthy"}]}`)
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	cat := catalogForTest(server.Client(), 1024, 1, nil)
	id := uuid.New()
	done := make(chan error, 1)
	go func() {
		_, err := cat.ListModels(t.Context(), id, model.KindOpenRouter, server.URL+"/blocked", "")
		done <- err
	}()
	<-entered
	original := cat.breakers[id]
	for range 300 {
		_, err := cat.ListModels(t.Context(), uuid.New(), model.KindOpenRouter, server.URL, "")
		require.NoError(t, err)
	}
	require.Same(t, original, cat.breakers[id], "in-flight provider circuit cannot be replaced")
	require.LessOrEqual(t, len(cat.breakers), 256)
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-done)
}

func TestCatalogStatusRetryAfterAndMalformedBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		want     errors.Code
		attempts int
	}{
		{"unauthorized", 401, secretKey, errors.CodeLLMProviderRejected, 1},
		{"rate limit", 429, secretKey, errors.CodeRateLimited, 2},
		{"server", 503, secretKey, errors.CodeProviderUnavailable, 2},
		{"malformed", 200, secretKey, errors.CodeLLMProviderRejected, 1},
		{"missing data", 200, `{}`, errors.CodeLLMProviderRejected, 1},
		{"trailing", 200, `{"data":[]}garbage`, errors.CodeLLMProviderRejected, 1},
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
			_, err := cat.ListModels(platformContext(context.Background()), uuid.New(), model.KindOpenRouter, srv.URL, secretKey)
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
	cat := NewHTTPModelCatalog(retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Second}), time.Second, retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 2, OpenTimeout: time.Second, HalfOpenMaxCalls: 1}))
	for _, endpoint := range []string{srv.URL, "https://username:" + secretKey + "@api.example.com/v1"} {
		_, err := cat.ListModels(platformContext(context.Background()), uuid.New(), model.KindOpenRouter, endpoint, secretKey)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretKey)
	}
}

func TestCatalogChunkedOversizeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, err := io.WriteString(w, `{"data":[],"padding":"`+strings.Repeat("x", 128)+`"}`)
		require.NoError(t, err)
	}))
	defer srv.Close()
	cat := catalogForTest(srv.Client(), 64, 1, nil)
	_, err := cat.ListModels(platformContext(context.Background()), uuid.New(), model.KindOpenRouter, srv.URL, secretKey)
	require.Equal(t, errors.CodeLLMProviderRejected, code(t, err))
}

func TestCatalogUnknownPricesRemainNullable(t *testing.T) {
	for _, value := range []string{"-1", "NaN", "Inf", "unknown"} {
		require.Nil(t, perMillion(value))
	}
}
