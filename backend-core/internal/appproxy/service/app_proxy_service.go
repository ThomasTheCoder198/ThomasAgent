package service

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

// Forwarder is satisfied by *ragx.Caller.
type Forwarder interface {
	Forward(ctx context.Context, in ragx.ForwardRequest) (ragx.ForwardResponse, error)
}

type AppProxyService interface {
	Forward(ctx context.Context, call model.AppCall) (model.AppResponse, error)
}

type AppProxyServiceImpl struct {
	forwarder Forwarder
}

func NewAppProxyService(forwarder Forwarder) AppProxyService {
	return &AppProxyServiceImpl{forwarder: forwarder}
}

// Spec §14.2: unreserved URL characters and slashes only; percent-decoding already happened in the router.
var appPathPattern = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)

const (
	pathSeparator  = "/"
	doubleSlash    = "//"
	parentSegment  = ".."
	currentSegment = "."
)

// Forward checks what the browser asked for and forwards it with a scope taken from the method. The org is
// already in ctx (RequireOrg); the token's oid comes only from there.
func (s *AppProxyServiceImpl) Forward(ctx context.Context, call model.AppCall) (model.AppResponse, error) {
	path, ok := cleanAppPath(call.Path)
	if !ok {
		return model.AppResponse{}, errors.ErrNotFound
	}
	scope, ok := model.ScopeFor(call.Method)
	if !ok {
		return model.AppResponse{}, errors.ErrMethodNotAllowed
	}
	if call.IdempotencyKey != "" && !httpx.ValidIdempotencyKey(call.IdempotencyKey) {
		return model.AppResponse{}, errors.FieldError(model.HeaderIdempotencyKey, errors.FieldCodeInvalid)
	}
	res, err := s.forwarder.Forward(ctx, ragx.ForwardRequest{
		Method: call.Method, Path: path, RawQuery: call.RawQuery, Route: call.Route, ContentType: call.ContentType,
		IdempotencyKey: call.IdempotencyKey, AcceptLanguage: call.AcceptLanguage, Body: call.Body, Scope: scope,
	})
	if err != nil {
		return model.AppResponse{}, err
	}
	return model.AppResponse{Status: res.Status, ContentType: res.ContentType, Body: res.Body}, nil
}

// cleanAppPath removes gin's leading slash. An empty path or a trailing slash is refused too (ruling P6): ragx
// would redirect it, which the error table would turn into a misleading 503. The double slash is checked on raw,
// so a leading "//" is not hidden by the trim.
// hasCurrentSegment finds a "." segment: JoinPath would drop it, so ragx would get a path other than the one checked.
func hasCurrentSegment(path string) bool {
	return slices.Contains(strings.Split(path, pathSeparator), currentSegment)
}

func cleanAppPath(raw string) (string, bool) {
	path := strings.TrimPrefix(raw, pathSeparator)
	if path == "" || strings.HasSuffix(path, pathSeparator) || strings.Contains(raw, doubleSlash) ||
		strings.Contains(path, parentSegment) || hasCurrentSegment(path) || !appPathPattern.MatchString(path) {
		return "", false
	}
	return path, true
}
