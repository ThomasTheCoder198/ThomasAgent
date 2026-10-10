package model

import (
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const (
	PathParamAppPath = "path"
	// HeaderIdempotencyKey is httpx's spelling, so the proxy and the access log never drift.
	HeaderIdempotencyKey = httpx.HeaderIdempotencyKey
)

// AppCall is what the proxy controller read from the browser's request; nothing else can be forwarded.
type AppCall struct {
	Method         string
	Path           string
	RawQuery       string
	Route          string
	ContentType    string
	IdempotencyKey string
	AcceptLanguage string
	Body           []byte
}

type AppResponse struct {
	Status      int
	ContentType string
	Body        []byte
}

// methodScopes is the one table behind both the registered methods and the token scope (ruling P7, P-F10c).
// OPTIONS would skip CSRF, and CONNECT/TRACE have no meaning for an app API, so they are not in it.
var methodScopes = []struct {
	method string
	scope  apptoken.Scope
}{
	{http.MethodGet, apptoken.ScopeKBRead},
	{http.MethodHead, apptoken.ScopeKBRead},
	{http.MethodPost, apptoken.ScopeKBWrite},
	{http.MethodPut, apptoken.ScopeKBWrite},
	{http.MethodPatch, apptoken.ScopeKBWrite},
	{http.MethodDelete, apptoken.ScopeKBWrite},
}

// ForwardedMethods are the methods the app proxy serves.
func ForwardedMethods() []string {
	methods := make([]string, 0, len(methodScopes))
	for _, entry := range methodScopes {
		methods = append(methods, entry.method)
	}
	return methods
}

// ScopeFor returns the App token scope for a method: reads get kb:read, everything else kb:write.
func ScopeFor(method string) (apptoken.Scope, bool) {
	for _, entry := range methodScopes {
		if entry.method == method {
			return entry.scope, true
		}
	}
	return "", false
}
