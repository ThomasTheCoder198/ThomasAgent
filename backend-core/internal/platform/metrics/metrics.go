package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	OutcomeSuccess    = "success"
	OutcomeError      = "error"
	OutcomeRetry      = "retry"
	OutcomeDeadLetter = "dead_letter"

	TokenKindInput     = "input"
	TokenKindOutput    = "output"
	TokenKindCached    = "cached"
	TokenKindReasoning = "reasoning"

	otherMethod        = "OTHER"
	statusClassDivisor = 100
	statusClassFormat  = "%dxx"
	exemplarRequestID  = "request_id"

	labelMethod      = "method"
	labelRoute       = "route"
	labelStatusClass = "status_class"
	labelProvider    = "provider"
	labelModel       = "model"
	labelRole        = "role"
	labelOutcome     = "outcome"
	labelKind        = "kind"
	labelTarget      = "target"
	labelStream      = "stream"

	httpRequestsName     = "http_server_requests_total"
	httpDurationName     = "http_server_request_duration_seconds"
	outboundRequestsName = "outbound_requests_total"
	outboundDurationName = "outbound_request_duration_seconds"
	outboxPendingName    = "outbox_pending"
	outboxLagName        = "outbox_relay_lag_seconds"
	jobsProcessedName    = "jobs_processed_total"
	jobsDeadLetteredName = "jobs_dlq_total"
	llmCallsName         = "llm_calls_total"
	llmTokensName        = "llm_tokens_total"
	llmCostName          = "llm_cost_usd_total"
	llmDurationName      = "llm_call_duration_seconds"
	llmTTFTName          = "llm_ttft_seconds"
)

// Unknown client verbs share one label so arbitrary text cannot add series.
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return otherMethod
	}
}

type LLMObservation struct {
	Provider, Model, Role, Outcome                           string
	InputTokens, OutputTokens, CachedTokens, ReasoningTokens int64
	CostUSD                                                  float64
	Duration, TTFT                                           time.Duration
}

// Metrics owns its registry; one instance is built at startup and injected, so tests never share global state.
type Metrics struct {
	registry         *prometheus.Registry
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	outboundRequests *prometheus.CounterVec
	outboundDuration *prometheus.HistogramVec
	outboxPending    prometheus.Gauge
	outboxLag        prometheus.Gauge
	jobsProcessed    *prometheus.CounterVec
	jobsDeadLettered *prometheus.CounterVec
	llmCalls         *prometheus.CounterVec
	llmTokens        *prometheus.CounterVec
	llmCost          *prometheus.CounterVec
	llmDuration      *prometheus.HistogramVec
	llmTTFT          *prometheus.HistogramVec
}

func New(httpBuckets, llmBuckets []float64) *Metrics {
	collector := &Metrics{registry: prometheus.NewRegistry()}
	collector.addHTTP(httpBuckets)
	collector.addJobs()
	collector.addLLM(llmBuckets)
	collector.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return collector
}

func (m *Metrics) addHTTP(buckets []float64) {
	m.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: httpRequestsName, Help: "HTTP requests served by core.",
	}, []string{labelMethod, labelRoute, labelStatusClass})
	m.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: httpDurationName, Help: "HTTP request duration in seconds.", Buckets: buckets,
	}, []string{labelMethod, labelRoute, labelStatusClass})
	m.outboundRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: outboundRequestsName, Help: "Requests core sent to providers and ragx.",
	}, []string{labelTarget, labelOutcome})
	m.outboundDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: outboundDurationName, Help: "Outbound request duration in seconds.", Buckets: buckets,
	}, []string{labelTarget, labelOutcome})
	m.registry.MustRegister(m.httpRequests, m.httpDuration, m.outboundRequests, m.outboundDuration)
}

func (m *Metrics) addJobs() {
	m.outboxPending = prometheus.NewGauge(prometheus.GaugeOpts{Name: outboxPendingName, Help: "Unpublished outbox rows."})
	m.outboxLag = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: outboxLagName, Help: "Age of the oldest unpublished outbox row in seconds.",
	})
	m.jobsProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: jobsProcessedName, Help: "Stream jobs handled, by outcome.",
	}, []string{labelStream, labelOutcome})
	m.jobsDeadLettered = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: jobsDeadLetteredName, Help: "Stream jobs moved to the dead-letter stream.",
	}, []string{labelStream})
	m.registry.MustRegister(m.outboxPending, m.outboxLag, m.jobsProcessed, m.jobsDeadLettered)
}

func (m *Metrics) addLLM(buckets []float64) {
	model := []string{labelProvider, labelModel, labelRole}
	m.llmCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: llmCallsName, Help: "Model calls, by outcome.",
	}, append(append([]string{}, model...), labelOutcome))
	m.llmTokens = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: llmTokensName, Help: "Model tokens, by kind.",
	}, append(append([]string{}, model...), labelKind))
	m.llmCost = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: llmCostName, Help: "Model cost in US dollars (llm_usage in Postgres is the source of truth).",
	}, model)
	m.llmDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: llmDurationName, Help: "Model call duration in seconds.", Buckets: buckets,
	}, model)
	m.llmTTFT = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: llmTTFTName, Help: "Time to first token in seconds.", Buckets: buckets,
	}, model)
	m.registry.MustRegister(m.llmCalls, m.llmTokens, m.llmCost, m.llmDuration, m.llmTTFT)
}

// Handler serves OpenMetrics so Prometheus receives the request_id exemplars.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (m *Metrics) ObserveHTTP(method, route string, status int, elapsed time.Duration, requestID string) {
	method = normalizeMethod(method)
	class := fmt.Sprintf(statusClassFormat, status/statusClassDivisor)
	m.httpRequests.WithLabelValues(method, route, class).Inc()
	observe(m.httpDuration.WithLabelValues(method, route, class), elapsed.Seconds(), requestID)
}

func (m *Metrics) ObserveOutbound(target, outcome string, elapsed time.Duration) {
	m.outboundRequests.WithLabelValues(target, outcome).Inc()
	m.outboundDuration.WithLabelValues(target, outcome).Observe(elapsed.Seconds())
}

// RegisterJobStream gives Prometheus a zero baseline before failures create the first increment.
func (m *Metrics) RegisterJobStream(stream string) {
	m.jobsDeadLettered.WithLabelValues(stream)
}

func (m *Metrics) ObserveJob(stream, outcome string) {
	m.jobsProcessed.WithLabelValues(stream, outcome).Inc()
	if outcome == OutcomeDeadLetter {
		m.jobsDeadLettered.WithLabelValues(stream).Inc()
	}
}

func (m *Metrics) SetOutboxBacklog(pending int, oldest time.Duration) {
	m.outboxPending.Set(float64(pending))
	m.outboxLag.Set(oldest.Seconds())
}

func (m *Metrics) ObserveLLMCall(observation LLMObservation) {
	m.llmCalls.WithLabelValues(observation.Provider, observation.Model, observation.Role, observation.Outcome).Inc()
	for kind, count := range map[string]int64{
		TokenKindInput: observation.InputTokens, TokenKindOutput: observation.OutputTokens, TokenKindCached: observation.CachedTokens, TokenKindReasoning: observation.ReasoningTokens,
	} {
		if count > 0 {
			m.llmTokens.WithLabelValues(observation.Provider, observation.Model, observation.Role, kind).Add(float64(count))
		}
	}
	if observation.CostUSD > 0 {
		m.llmCost.WithLabelValues(observation.Provider, observation.Model, observation.Role).Add(observation.CostUSD)
	}
	m.llmDuration.WithLabelValues(observation.Provider, observation.Model, observation.Role).Observe(observation.Duration.Seconds())
	if observation.TTFT > 0 {
		m.llmTTFT.WithLabelValues(observation.Provider, observation.Model, observation.Role).Observe(observation.TTFT.Seconds())
	}
}

func observe(observer prometheus.Observer, value float64, requestID string) {
	if exemplar, ok := observer.(prometheus.ExemplarObserver); ok && requestID != "" {
		exemplar.ObserveWithExemplar(value, prometheus.Labels{exemplarRequestID: requestID})
		return
	}
	observer.Observe(value)
}
