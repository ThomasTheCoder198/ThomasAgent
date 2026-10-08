package client

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

const (
	headerAuthorization = "Authorization"
	bearerPrefix        = "Bearer "
	headerRetryAfter    = "Retry-After"
	modelsPath          = "/models"
	catalogOperation    = "provider.list_models"
	catalogSuccess      = "success"
)

// ModelCatalog lists the models a provider publishes at GET /models.
type ModelCatalog interface {
	ListModels(ctx context.Context, providerID uuid.UUID, kind model.ProviderKind, baseURL, apiKey string) ([]model.RemoteModel, error)
}

// OutboundRecorder receives one observation per HTTP attempt; metrics.Metrics implements it.
type OutboundRecorder interface {
	ObserveOutbound(target, outcome string, elapsed time.Duration)
}

// HTTPModelCatalog calls a provider's GET /models with retry, a circuit breaker per provider
// and a response size cap. Only OpenRouter responses carry capabilities and prices.
type HTTPModelCatalog struct {
	Client           *http.Client
	Policy           retry.Policy
	Breaker          *retry.Breaker
	MaxBreakers      int
	MaxResponseBytes int64
	Metrics          OutboundRecorder
	log              *slog.Logger
	initErr          error
	breakers         map[uuid.UUID]*providerBreaker
	breakerMu        sync.Mutex
	breakerClock     uint64
}

// NewHTTPModelCatalog builds a catalog with a guarded public-only client; tests and tools use it.
func NewHTTPModelCatalog(policy retry.Policy, timeout time.Duration, breaker *retry.Breaker) *HTTPModelCatalog {
	httpClient, err := outbound.NewClient(timeout, nil)
	catalog := NewHTTPModelCatalogWithClient(policy, httpClient, config.DefaultProviderMaxResponseBytes, breaker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		catalog.initErr = errors.ErrInternalError
	}
	return catalog
}

func NewHTTPModelCatalogWithClient(policy retry.Policy, httpClient *http.Client, maxBytes int64, breaker *retry.Breaker, log *slog.Logger, maxBreakers ...int) *HTTPModelCatalog {
	limit := config.DefaultProviderMaxBreakers
	if len(maxBreakers) > 0 {
		limit = maxBreakers[0]
	}
	return &HTTPModelCatalog{Client: httpClient, Policy: policy, Breaker: breaker, MaxResponseBytes: maxBytes, MaxBreakers: limit, log: log}
}

func (c *HTTPModelCatalog) ListModels(ctx context.Context, providerID uuid.UUID, kind model.ProviderKind, baseURL, apiKey string) ([]model.RemoteModel, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	if !c.isConfigured() {
		return nil, errors.ErrInternalError
	}
	entry, err := c.acquireBreaker(providerID)
	if err != nil {
		return nil, err
	}
	defer c.releaseBreaker(entry)
	var body openRouterModels
	err = retry.Do(ctx, c.Policy, func(ctx context.Context) error {
		return entry.breaker.Execute(ctx, func(ctx context.Context) error { return c.fetch(ctx, kind, baseURL, apiKey, &body) })
	})
	if err != nil {
		return nil, err
	}
	if kind != model.KindOpenRouter {
		return toPlainModels(body), nil
	}
	return toOpenRouterModels(body), nil
}

func (c *HTTPModelCatalog) isConfigured() bool {
	return c.Client != nil && c.Breaker != nil && c.MaxResponseBytes > 0 && c.MaxResponseBytes < int64(maxRetryAfter) && c.Policy.MaxAttempts > 0 && c.MaxBreakers > 0
}

func (c *HTTPModelCatalog) fetch(ctx context.Context, kind model.ProviderKind, baseURL, apiKey string, out *openRouterModels) (err error) {
	start := time.Now()
	defer func() { c.observe(ctx, kind, err, time.Since(start)) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+modelsPath, nil)
	if err != nil {
		return errors.ErrLLMProviderRejected
	}
	if apiKey != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+apiKey)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		if stderrors.Is(err, errors.ErrLLMProviderRejected) {
			return errors.ErrLLMProviderRejected
		}
		if ctx.Err() != nil {
			return errors.ErrProviderUnavailable.WithCause(ctx.Err())
		}
		// net/http attaches the raw URL to errors, so only the catalog code crosses this boundary.
		return errors.ErrProviderUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	if err = errorForStatus(res.StatusCode); err != nil {
		return withRetryAfter(err, res.Header.Get(headerRetryAfter))
	}
	return c.decodeModels(res.Body, out)
}

func (c *HTTPModelCatalog) observe(ctx context.Context, kind model.ProviderKind, err error, elapsed time.Duration) {
	if c.Metrics != nil {
		outcome := metrics.OutcomeSuccess
		if err != nil {
			outcome = metrics.OutcomeError
		}
		c.Metrics.ObserveOutbound(string(kind), outcome, elapsed)
	}
	if c.log != nil {
		c.log.InfoContext(ctx, catalogOperation, slog.Bool(catalogSuccess, err == nil), slog.Int64(logging.FieldDurationMS, elapsed.Milliseconds()))
	}
}

func (c *HTTPModelCatalog) decodeModels(body io.Reader, out *openRouterModels) error {
	raw, err := io.ReadAll(io.LimitReader(body, c.MaxResponseBytes+1))
	if err != nil {
		return errors.ErrProviderUnavailable
	}
	if int64(len(raw)) > c.MaxResponseBytes {
		return errors.ErrLLMProviderRejected
	}
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Data == nil {
		return errors.ErrLLMProviderRejected
	}
	out.Data = make([]openRouterModel, len(envelope.Data))
	for index, entry := range envelope.Data {
		var decoded openRouterModel
		if err := json.Unmarshal(entry, &decoded); err == nil {
			out.Data[index] = decoded
		}
		// A zero model is rejected and counted by sync without losing valid siblings.
	}
	return nil
}
