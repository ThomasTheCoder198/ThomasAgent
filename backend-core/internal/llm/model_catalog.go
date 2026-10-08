package llm

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

const (
	headerAuthorization = "Authorization"
	bearerScheme        = "Bearer"
	bearerPrefix        = bearerScheme + " "
	headerRetryAfter    = "Retry-After"
	catalogOperation    = "provider.list_models"
	catalogSuccess      = "success"
	decimalBase         = 10
	intBits             = 64
	maxRetryAfter       = time.Duration(1<<63 - 1)

	modelsPath           = "/models"
	tokensPerMillion     = 1_000_000
	openRouterParamTool  = "tools"
	openRouterParamThink = "reasoning"
	modalityImage        = "image"
	modalityText         = "text"
)

type OutboundRecorder interface {
	ObserveOutbound(target, outcome string, elapsed time.Duration)
}

// HTTPModelCatalog fetches a provider's GET /models list with retry, a circuit breaker per
// provider and a response size cap. Only OpenRouter responses carry capabilities and prices.
type HTTPModelCatalog struct {
	Client           *http.Client
	Policy           retry.Policy
	Breaker          *retry.Breaker
	breakers         map[uuid.UUID]*providerBreaker
	breakerMu        sync.Mutex
	MaxBreakers      int
	breakerClock     uint64
	MaxResponseBytes int64
	log              *slog.Logger
	initErr          error
	Metrics          OutboundRecorder
}

type providerBreaker struct {
	breaker  *retry.Breaker
	active   int
	lastUsed uint64
}

type openRouterModels struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Architecture  struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters []string `json:"supported_parameters"`
}

type statusError struct{ status int }

func (e statusError) Error() string { return "provider returned status " + strconv.Itoa(e.status) }

func (c *HTTPModelCatalog) ListModels(ctx context.Context, providerID uuid.UUID, kind ProviderKind, baseURL, apiKey string) ([]RemoteModel, error) {
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
	if kind != KindOpenRouter {
		return toPlainModels(body), nil
	}
	return toOpenRouterModels(body), nil
}

func (c *HTTPModelCatalog) isConfigured() bool {
	return c.Client != nil && c.Breaker != nil && c.MaxResponseBytes > 0 && c.MaxResponseBytes < int64(maxRetryAfter) && c.Policy.MaxAttempts > 0 && c.MaxBreakers > 0
}

func (c *HTTPModelCatalog) acquireBreaker(providerID uuid.UUID) (*providerBreaker, error) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if c.breakers == nil {
		c.breakers = make(map[uuid.UUID]*providerBreaker)
	}
	entry, exists := c.breakers[providerID]
	if !exists {
		if len(c.breakers) >= c.MaxBreakers && !c.discardIdleBreaker() {
			return nil, errors.ErrProviderUnavailable
		}
		entry = &providerBreaker{breaker: c.Breaker.ForProvider("provider-" + providerID.String())}
		c.breakers[providerID] = entry
	}
	c.breakerClock++
	entry.lastUsed = c.breakerClock
	entry.active++
	return entry, nil
}

func (c *HTTPModelCatalog) releaseBreaker(entry *providerBreaker) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	entry.active--
}

func (c *HTTPModelCatalog) discardIdleBreaker() bool {
	var oldest *providerBreaker
	var oldestID uuid.UUID
	for id, entry := range c.breakers {
		// Resetting a failing or in-flight circuit would bypass provider protection.
		if entry.active == 0 && entry.breaker.CanDiscard() && (oldest == nil || entry.lastUsed < oldest.lastUsed) {
			oldest, oldestID = entry, id
		}
	}
	if oldest == nil {
		return false
	}
	delete(c.breakers, oldestID)
	return true
}

func (c *HTTPModelCatalog) fetch(ctx context.Context, kind ProviderKind, baseURL, apiKey string, out *openRouterModels) (err error) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
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
	}()
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
	for i, entry := range envelope.Data {
		var model openRouterModel
		if err := json.Unmarshal(entry, &model); err == nil {
			out.Data[i] = model
		}
		// A zero model is rejected and counted by sync without losing valid siblings.
	}
	return nil
}

type retryAfterError struct {
	error
	delay time.Duration
}

func (e retryAfterError) RetryAfter() time.Duration { return e.delay }
func (e retryAfterError) Unwrap() error             { return e.error }
func withRetryAfter(err error, value string) error {
	if !retry.IsRetryable(err) {
		return err
	}
	if seconds, parseErr := strconv.ParseInt(value, decimalBase, intBits); parseErr == nil && seconds > 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return retryAfterError{err, maxRetryAfter}
		}
		return retryAfterError{err, time.Duration(seconds) * time.Second}
	}
	if deadline, parseErr := http.ParseTime(value); parseErr == nil {
		if delay := time.Until(deadline); delay > 0 {
			return retryAfterError{err, min(delay, maxRetryAfter)}
		}
	}
	return err
}

// 401/403/404 and other 4xx mean the key or base URL is wrong; split out to keep fetch under the complexity limit.
func errorForStatus(status int) error {
	switch {
	case status == http.StatusTooManyRequests:
		return errors.ErrRateLimited
	case status >= http.StatusInternalServerError:
		return errors.ErrProviderUnavailable.WithCause(statusError{status})
	case status >= http.StatusBadRequest:
		return errors.ErrLLMProviderRejected
	}
	return nil
}

func toPlainModels(body openRouterModels) []RemoteModel {
	out := make([]RemoteModel, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, RemoteModel{ModelRef: m.ID, DisplayName: m.ID})
	}
	return out
}

func toOpenRouterModels(body openRouterModels) []RemoteModel {
	out := make([]RemoteModel, 0, len(body.Data))
	for _, m := range body.Data {
		capabilities := []Capability{}
		if slices.Contains(m.Architecture.OutputModalities, modalityText) {
			capabilities = append(capabilities, CapabilityChat)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamTool) {
			capabilities = append(capabilities, CapabilityTools)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamThink) {
			capabilities = append(capabilities, CapabilityReasoning)
		}
		if slices.Contains(m.Architecture.InputModalities, modalityImage) {
			capabilities = append(capabilities, CapabilityVision)
		}
		var contextWindow *int
		if m.ContextLength > 0 {
			length := m.ContextLength
			contextWindow = &length
		}
		out = append(out, RemoteModel{
			ModelRef: m.ID, DisplayName: m.Name, Capabilities: capabilities, ContextWindow: contextWindow,
			InputPricePerMTok: perMillion(m.Pricing.Prompt), OutputPricePerMTok: perMillion(m.Pricing.Completion),
		})
	}
	return out
}

func perMillion(perToken string) *float64 {
	pricePerToken, err := strconv.ParseFloat(perToken, 64)
	if err != nil || pricePerToken < 0 || math.IsNaN(pricePerToken) || math.IsInf(pricePerToken, 0) {
		return nil
	}
	price := pricePerToken * tokensPerMillion
	if math.IsInf(price, 0) {
		return nil
	}
	return &price
}

func NewHTTPModelCatalog(policy retry.Policy, timeout time.Duration, breaker *retry.Breaker) *HTTPModelCatalog {
	client, err := outbound.NewClient(timeout, nil)
	c := NewHTTPModelCatalogWithClient(policy, client, config.DefaultProviderMaxResponseBytes, breaker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		c.initErr = errors.ErrInternalError
	}
	return c
}

func NewHTTPModelCatalogWithClient(policy retry.Policy, client *http.Client, maxBytes int64, breaker *retry.Breaker, log *slog.Logger, maxBreakers ...int) *HTTPModelCatalog {
	limit := config.DefaultProviderMaxBreakers
	if len(maxBreakers) > 0 {
		limit = maxBreakers[0]
	}
	return &HTTPModelCatalog{Client: client, Policy: policy, Breaker: breaker, MaxResponseBytes: maxBytes, MaxBreakers: limit, log: log}
}
