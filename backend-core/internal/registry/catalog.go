package registry

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"math"
	"strings"

	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

const (
	headerAuthorization = "Authorization"
	bearerPrefix        = "Bearer "
	headerRetryAfter    = "Retry-After"
	catalogTracer       = "core.registry"
	catalogOperation    = "provider.list_models"
	catalogFailure      = "provider request failed"
	catalogSuccess      = "success"
	catalogTraceID      = "trace_id"
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

type RemoteModel struct {
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}

type Catalog interface {
	ListModels(ctx context.Context, kind ProviderKind, baseURL, apiKey string) ([]RemoteModel, error)
}

type HTTPCatalog struct {
	Client           *http.Client
	Policy           retry.Policy
	Breaker          *retry.Breaker
	MaxResponseBytes int64
	log              *slog.Logger
	initErr          error
}

type openRouterModels struct {
	Data []struct {
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
	} `json:"data"`
}

type statusError struct{ status int }

func (e statusError) Error() string { return "provider returned status " + strconv.Itoa(e.status) }

func (c *HTTPCatalog) ListModels(ctx context.Context, kind ProviderKind, baseURL, apiKey string) ([]RemoteModel, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	if c.Client == nil || c.Breaker == nil || c.MaxResponseBytes <= 0 || c.MaxResponseBytes >= int64(maxRetryAfter) || c.Policy.MaxAttempts <= 0 {
		return nil, errors.ErrInternalError
	}
	var body openRouterModels
	err := retry.Do(ctx, c.Policy, func(ctx context.Context) error {
		return c.Breaker.Execute(ctx, func(ctx context.Context) error { return c.fetch(ctx, baseURL, apiKey, &body) })
	})
	if err != nil {
		return nil, err
	}
	if kind != KindOpenRouter {
		return toPlainModels(body), nil
	}
	return toOpenRouterModels(body), nil
}

func (c *HTTPCatalog) fetch(ctx context.Context, baseURL, apiKey string, out *openRouterModels) (err error) {
	ctx, span := otel.Tracer(catalogTracer).Start(ctx, catalogOperation, trace.WithSpanKind(trace.SpanKindClient))
	defer func() {
		defer span.End()
		if err != nil {
			span.SetStatus(codes.Error, catalogFailure)
		}
		if c.log != nil {
			c.log.InfoContext(ctx, catalogOperation, slog.Bool(catalogSuccess, err == nil), slog.String(catalogTraceID, span.SpanContext().TraceID().String()))
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+modelsPath, nil)
	if err != nil {
		return errors.ErrRegistryProviderRejected
	}
	if apiKey != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+apiKey)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		if stderrors.Is(err, errors.ErrRegistryProviderRejected) {
			return errors.ErrRegistryProviderRejected
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

func (c *HTTPCatalog) decodeModels(body io.Reader, out *openRouterModels) error {
	raw, err := io.ReadAll(io.LimitReader(body, c.MaxResponseBytes+1))
	if err != nil {
		return errors.ErrProviderUnavailable
	}
	if int64(len(raw)) > c.MaxResponseBytes {
		return errors.ErrRegistryProviderRejected
	}
	if err := json.Unmarshal(raw, out); err != nil || out.Data == nil {
		return errors.ErrRegistryProviderRejected
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
		return errors.New(errors.CodeRateLimited)
	case status >= http.StatusInternalServerError:
		return errors.New(errors.CodeProviderUnavailable, errors.WithCause(statusError{status}))
	case status >= http.StatusBadRequest:
		return errors.New(errors.CodeRegistryProviderRejected)
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
		caps := []Capability{}
		if slices.Contains(m.Architecture.OutputModalities, modalityText) {
			caps = append(caps, CapChat)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamTool) {
			caps = append(caps, CapTools)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamThink) {
			caps = append(caps, CapReasoning)
		}
		if slices.Contains(m.Architecture.InputModalities, modalityImage) {
			caps = append(caps, CapVision)
		}
		ctxLen := m.ContextLength
		out = append(out, RemoteModel{
			ModelRef: m.ID, DisplayName: m.Name, Capabilities: caps, ContextWindow: &ctxLen,
			InputPricePerMTok: perMillion(m.Pricing.Prompt), OutputPricePerMTok: perMillion(m.Pricing.Completion),
		})
	}
	return out
}

func perMillion(perToken string) *float64 {
	v, err := strconv.ParseFloat(perToken, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	price := v * tokensPerMillion
	if math.IsInf(price, 0) {
		return nil
	}
	return &price
}

func NewHTTPCatalog(policy retry.Policy, timeout time.Duration, breaker *retry.Breaker) *HTTPCatalog {
	client, err := outbound.NewClient(timeout, nil)
	c := NewHTTPCatalogWithClient(policy, client, config.DefaultProviderMaxResponseBytes, breaker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		c.initErr = errors.ErrInternalError
	}
	return c
}

func NewHTTPCatalogWithClient(policy retry.Policy, client *http.Client, maxBytes int64, breaker *retry.Breaker, log *slog.Logger) *HTTPCatalog {
	return &HTTPCatalog{Client: client, Policy: policy, Breaker: breaker, MaxResponseBytes: maxBytes, log: log}
}
