package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

type recordedOutbound struct{ target, outcome string }

type fakeOutbound struct{ calls []recordedOutbound }

func (f *fakeOutbound) ObserveOutbound(target, outcome string, _ time.Duration) {
	f.calls = append(f.calls, recordedOutbound{target, outcome})
}

func TestCatalog_RecordsOutboundByProviderKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	fake := &fakeOutbound{}
	policy := retry.Policy{RetryConfig: config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}}
	catalog := NewHTTPModelCatalogWithClient(policy, server.Client(), config.DefaultProviderMaxResponseBytes,
		retry.NewBreaker("test", config.BreakerConfig{FailureThreshold: 5, OpenTimeout: time.Second, HalfOpenMaxCalls: 1}), nil)
	catalog.Metrics = fake
	_, err := catalog.ListModels(context.Background(), uuid.New(), model.KindOpenRouter, server.URL, "")
	require.NoError(t, err)
	success := recordedOutbound{target: string(model.KindOpenRouter), outcome: metrics.OutcomeSuccess}
	require.Equal(t, []recordedOutbound{success, success}, fake.calls, "the chat list and the decision list")
}
