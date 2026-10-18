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
	headerGoogleAPIKey  = "x-goog-api-key"
	modelsPath          = "/models"
	decisionsQuery      = "output_modalities=decisions"
	googleModelsQuery   = "pageSize=1000"
	catalogOperation    = "provider.list_models"
	catalogSuccess      = "success"
	fieldDecisionsList  = "decisions"
)

// ModelCatalog lists the models a provider publishes at its model list endpoint.
type ModelCatalog interface {
	ListModels(ctx context.Context, providerID uuid.UUID, kind model.ProviderKind, baseURL, apiKey string) ([]model.RemoteModel, error)
}

// OutboundRecorder receives one observation per HTTP attempt; metrics.Metrics implements it.
type OutboundRecorder interface {
	ObserveOutbound(target, outcome string, elapsed time.Duration)
}

// HTTPModelCatalog calls a provider's model list with retry, a circuit breaker per provider
// and a response size cap. OpenRouter and Google lists carry model facts (O8); other kinds give names only.
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
	guarded := func(req listRequest, decode func(io.Reader) error) error {
		return retry.Do(ctx, c.Policy, func(ctx context.Context) error {
			return entry.breaker.Execute(ctx, func(ctx context.Context) error { return c.fetch(ctx, kind, req, decode) })
		})
	}
	if kind == model.KindGoogle {
		var body googleModels
		if err := guarded(googleRequest(baseURL, apiKey), c.decodeGoogle(&body)); err != nil {
			return nil, err
		}
		return toGoogleModels(body), nil
	}
	var body openRouterModels
	if err := guarded(bearerRequest(baseURL, "", apiKey), c.decodeData(&body)); err != nil {
		return nil, err
	}
	if kind != model.KindOpenRouter {
		return toPlainModels(body), nil
	}
	return c.withDecisionModels(ctx, kind, baseURL, apiKey, toOpenRouterModels(body)), nil
}

// withDecisionModels adds OpenRouter's decision models (ruling A2-5). The list is alpha: one attempt, outside the
// breaker, and a failure only logs, so it never fails or trips a sync of the chat models.
func (c *HTTPModelCatalog) withDecisionModels(ctx context.Context, kind model.ProviderKind, baseURL, apiKey string, models []model.RemoteModel) []model.RemoteModel {
	var body openRouterModels
	if err := c.fetch(ctx, kind, bearerRequest(baseURL, decisionsQuery, apiKey), c.decodeData(&body)); err != nil {
		if c.log != nil {
			c.log.WarnContext(ctx, catalogOperation, slog.Bool(fieldDecisionsList, false), slog.String(logging.FieldErrorCode, string(errors.CodeOf(err))))
		}
		return models
	}
	return mergeDecisionModels(models, toDecisionModels(body))
}

// listRequest is one GET of a provider's model list; the key is in a header and never in the URL.
type listRequest struct {
	url    string
	header http.Header
}

func bearerRequest(baseURL, rawQuery, apiKey string) listRequest {
	target := strings.TrimRight(baseURL, "/") + modelsPath
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	header := http.Header{}
	if apiKey != "" {
		header.Set(headerAuthorization, bearerPrefix+apiKey)
	}
	return listRequest{url: target, header: header}
}

func googleRequest(baseURL, apiKey string) listRequest {
	header := http.Header{}
	if apiKey != "" {
		header.Set(headerGoogleAPIKey, apiKey)
	}
	return listRequest{url: strings.TrimRight(baseURL, "/") + modelsPath + "?" + googleModelsQuery, header: header}
}

func (c *HTTPModelCatalog) isConfigured() bool {
	return c.Client != nil && c.Breaker != nil && c.MaxResponseBytes > 0 && c.MaxResponseBytes < int64(maxRetryAfter) && c.Policy.MaxAttempts > 0 && c.MaxBreakers > 0
}

func (c *HTTPModelCatalog) fetch(ctx context.Context, kind model.ProviderKind, list listRequest, decode func(io.Reader) error) (err error) {
	start := time.Now()
	defer func() { c.observe(ctx, kind, err, time.Since(start)) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, list.url, nil)
	if err != nil {
		return errors.ErrLLMProviderRejected
	}
	for name, values := range list.header {
		req.Header[name] = values
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
	return decode(res.Body)
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

// readCapped reads at most MaxResponseBytes; a larger body is the provider's fault, not ours.
func (c *HTTPModelCatalog) readCapped(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, c.MaxResponseBytes+1))
	if err != nil {
		return nil, errors.ErrProviderUnavailable
	}
	if int64(len(raw)) > c.MaxResponseBytes {
		return nil, errors.ErrLLMProviderRejected
	}
	return raw, nil
}

func (c *HTTPModelCatalog) decodeData(out *openRouterModels) func(io.Reader) error {
	return func(body io.Reader) error {
		raw, err := c.readCapped(body)
		if err != nil {
			return err
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
}

func (c *HTTPModelCatalog) decodeGoogle(out *googleModels) func(io.Reader) error {
	return func(body io.Reader) error {
		raw, err := c.readCapped(body)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, out); err != nil || out.Models == nil {
			return errors.ErrLLMProviderRejected
		}
		return nil
	}
}
