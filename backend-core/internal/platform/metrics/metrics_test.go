package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

var (
	testHTTPBuckets = []float64{0.01, 0.1, 1}
	testLLMBuckets  = []float64{1, 10}
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return string(body)
}

func TestObserveHTTP_CountsByRouteAndStatusClassWithExemplar(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	m.ObserveHTTP(http.MethodGet, "/api/v1/providers/{id}", http.StatusNotFound, 30*time.Millisecond, "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
	require.InDelta(t, 1, testutil.ToFloat64(m.httpRequests.WithLabelValues("GET", "/api/v1/providers/{id}", "4xx")), 0)
	body := scrape(t, m)
	require.Contains(t, body, `http_server_request_duration_seconds_bucket{method="GET",route="/api/v1/providers/{id}",status_class="4xx"`)
	require.Contains(t, body, `request_id="req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"`)
	require.NotContains(t, body, `request_id="req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21",route`, "request id is an exemplar, never a label")
}

func TestObserveHTTP_BoundsLabels(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	m.ObserveHTTP("FOO", "unmatched", http.StatusMethodNotAllowed, time.Millisecond, "")
	m.ObserveHTTP("BAR", "unmatched", http.StatusMethodNotAllowed, time.Millisecond, "")
	require.InDelta(t, 2, testutil.ToFloat64(m.httpRequests.WithLabelValues("OTHER", "unmatched", "4xx")), 0)
	require.Equal(t, 1, testutil.CollectAndCount(m.httpRequests))
}

func TestObserveOutboundAndJobs(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	m.ObserveOutbound("openrouter", OutcomeError, time.Second)
	m.ObserveJob("thomas.ingest.requested", OutcomeSuccess)
	m.ObserveJob("thomas.ingest.requested", OutcomeDeadLetter)
	m.SetOutboxBacklog(3, 90*time.Second)
	require.InDelta(t, 1, testutil.ToFloat64(m.outboundRequests.WithLabelValues("openrouter", OutcomeError)), 0)
	require.InDelta(t, 1, testutil.ToFloat64(m.jobsProcessed.WithLabelValues("thomas.ingest.requested", OutcomeDeadLetter)), 0)
	require.InDelta(t, 1, testutil.ToFloat64(m.jobsDeadLettered.WithLabelValues("thomas.ingest.requested")), 0)
	require.InDelta(t, 3, testutil.ToFloat64(m.outboxPending), 0)
	require.InDelta(t, 90, testutil.ToFloat64(m.outboxLag), 0)
}

func TestRegisterJobStream_ExportsDLQBaselineBeforeFirstFailure(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	const stream = "thomas.ingest.requested"
	m.RegisterJobStream(stream)
	require.Contains(t, scrape(t, m), `jobs_dlq_total{stream="thomas.ingest.requested"} 0`)
	m.ObserveJob(stream, OutcomeDeadLetter)
	require.Contains(t, scrape(t, m), `jobs_dlq_total{stream="thomas.ingest.requested"} 1`)
	m.RegisterJobStream(stream)
	require.Contains(t, scrape(t, m), `jobs_dlq_total{stream="thomas.ingest.requested"} 1`, "registering an existing group must not reset its counter")
}

func TestObserveLLMCall_RecordsTokensCostAndLatency(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	m.ObserveLLMCall(LLMObservation{
		Provider: "openrouter", Model: "anthropic/claude-sonnet-4.5", Role: "chat.default", Outcome: OutcomeSuccess,
		InputTokens: 100, OutputTokens: 20, CachedTokens: 50, ReasoningTokens: 0, CostUSD: 0.0125,
		Duration: 3 * time.Second, TTFT: 700 * time.Millisecond,
	})
	labels := []string{"openrouter", "anthropic/claude-sonnet-4.5", "chat.default"}
	require.InDelta(t, 1, testutil.ToFloat64(m.llmCalls.WithLabelValues(append(labels, OutcomeSuccess)...)), 0)
	require.InDelta(t, 100, testutil.ToFloat64(m.llmTokens.WithLabelValues(append(labels, TokenKindInput)...)), 0)
	require.InDelta(t, 50, testutil.ToFloat64(m.llmTokens.WithLabelValues(append(labels, TokenKindCached)...)), 0)
	require.InDelta(t, 0.0125, testutil.ToFloat64(m.llmCost.WithLabelValues(labels...)), 1e-12)
	require.True(t, strings.Contains(scrape(t, m), "llm_ttft_seconds_bucket"))
}

func TestHandler_IncludesRuntimeCollectors(t *testing.T) {
	body := scrape(t, New(testHTTPBuckets, testLLMBuckets))
	require.Contains(t, body, "go_goroutines")
	require.Contains(t, body, "process_")
}

func TestObserveChatRunQueue_RecordsTheWaitWithoutLabels(t *testing.T) {
	m := New(testHTTPBuckets, testLLMBuckets)
	m.ObserveChatRunQueue(250 * time.Millisecond)
	body := scrape(t, m)
	require.Contains(t, body, "chat_run_queue_seconds_count 1")
	require.Contains(t, body, "chat_run_queue_seconds_sum 0.25")
}
