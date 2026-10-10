package ragx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

// ForwardRequest is everything the app proxy may send to ragx; anything not here (cookies, the browser's
// Authorization, unknown headers) cannot reach ragx. Path is already validated and has no leading slash.
type ForwardRequest struct {
	Method         string
	Path           string
	RawQuery       string
	Route          string
	ContentType    string
	IdempotencyKey string
	AcceptLanguage string
	Body           []byte
	Scope          apptoken.Scope
}

type ForwardResponse struct {
	Status      int
	ContentType string
	Body        []byte
}

// Forward is the generic data plane behind core's app proxy (spec §14.2): it does not know ragx's routes. Its
// deadline is the caller's (the HTTP route's), not RequestTimeout.
func (c *Caller) Forward(ctx context.Context, in ForwardRequest) (ForwardResponse, error) {
	cl, err := c.begin(ctx, opForward, in.Scope, false)
	if err != nil {
		return ForwardResponse{}, err
	}
	cl.method, cl.route = in.Method, httpx.ContractRoute(in.Route)
	target := c.base.JoinPath(apiPrefix, in.Path)
	target.RawQuery = in.RawQuery
	req, err := http.NewRequestWithContext(cl.ctx, in.Method, target.String(), bytes.NewReader(in.Body))
	if err != nil {
		return ForwardResponse{}, errors.ErrInternalError.WithCause(fmt.Errorf("build ragx request: %w", err))
	}
	setIfPresent(req.Header, httpx.HeaderContentType, in.ContentType)
	setIfPresent(req.Header, httpx.HeaderIdempotencyKey, in.IdempotencyKey)
	setIfPresent(req.Header, httpx.HeaderAcceptLanguage, in.AcceptLanguage)
	for _, edit := range cl.editors {
		if err := edit(cl.ctx, req); err != nil {
			return ForwardResponse{}, errors.ErrInternalError.WithCause(err)
		}
	}
	res, err := c.http.Do(req) //nolint:bodyclose // readForward closes the body on every path
	out, failure := c.readForward(res, err)
	status, traceID := 0, ""
	if res != nil {
		status, traceID = res.StatusCode, res.Header.Get(outbound.HeaderRagxTraceID)
	}
	c.logCall(cl, status, traceID, failure)
	return out, failure
}

func setIfPresent(header http.Header, name, value string) {
	if value != "" {
		header.Set(name, value)
	}
}

// readForward buffers at most MaxResponseBytes+1 before anything is written to the browser, so an oversized body
// can still become a clean 503.
func (c *Caller) readForward(res *http.Response, err error) (ForwardResponse, error) {
	if err != nil {
		return ForwardResponse{}, errors.ErrRagxUnavailable.WithCause(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, c.cfg.MaxResponseBytes+1))
	if err != nil {
		return ForwardResponse{}, errors.ErrRagxUnavailable.WithCause(fmt.Errorf("read ragx response: %w", err))
	}
	if int64(len(body)) > c.cfg.MaxResponseBytes {
		return ForwardResponse{}, errors.ErrRagxUnavailable.WithCause(errors.ErrRagxResponseTooLarge)
	}
	if failure := classifyForward(res.StatusCode, body); failure != nil {
		return ForwardResponse{}, failure
	}
	if len(body) == 0 {
		body = nil // an empty body is no body: 204 and HEAD answers stay nil for the caller
	}
	return ForwardResponse{Status: res.StatusCode, ContentType: res.Header.Get(httpx.HeaderContentType), Body: body}, nil
}

// classifyForward is spec §14.2's table. A 403 means the route needs a scope the UI never gets (org:provision,
// tool:*), so to the browser that route does not exist.
func classifyForward(status int, body []byte) error {
	cause := fmt.Errorf("ragx answered %d", status)
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized:
		return errors.ErrRagxUnavailable.WithCause(cause)
	case status == http.StatusForbidden:
		return errors.ErrNotFound.WithCause(cause)
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError && isCatalogError(status, body):
		return nil
	default:
		return errors.ErrRagxUnavailable.WithCause(cause)
	}
}

func isCatalogError(status int, body []byte) bool {
	var parsed errorBody
	if json.Unmarshal(body, &parsed) != nil {
		return false
	}
	_, ok := catalogCode(status, parsed)
	return ok
}
