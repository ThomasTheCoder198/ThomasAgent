package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	llmprovidercontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const openAPIPath = "../../../contracts/openapi/core.v1.yaml"

var routeParam = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_]*)`)

func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(openAPIPath)
	require.NoError(t, err)
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	out := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			if method != "parameters" {
				out[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	return out
}

func TestRoutesDocumentedInOpenAPI(t *testing.T) {
	r := httpx.NewRouter(nil)
	registerRoutes(r, controllers{
		auth:          authcontroller.NewAuthController(authcontroller.AuthControllerParams{}),
		provider:      llmprovidercontroller.NewProviderController(nil, 1),
		model:         llmprovidercontroller.NewModelController(nil, 1),
		modelRole:     llmprovidercontroller.NewModelRoleController(nil, 1),
		internalModel: llmprovidercontroller.NewInternalModelController(nil),
	}, middlewares{requestTimeout: time.Second})
	documented := documentedRoutes(t)
	implemented := map[string]bool{}
	var missing []string
	for _, route := range r.Routes() {
		key := route.Method + " " + routeParam.ReplaceAllString(route.Path, "{$1}")
		if !documented[key] {
			missing = append(missing, key)
		}
		implemented[key] = true
	}
	sort.Strings(missing)
	require.Empty(t, missing, "add these routes to contracts/openapi/core.v1.yaml, then run task gen")
	var stale []string
	for key := range documented {
		if !implemented[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	require.Empty(t, stale, "remove stale paths from contracts/openapi/core.v1.yaml")
}

func TestRoutes_NoProviderRedirectAndEnvelopedErrors(t *testing.T) {
	r := httpx.NewRouter(nil)
	registerRoutes(r, controllers{
		auth:          authcontroller.NewAuthController(authcontroller.AuthControllerParams{}),
		provider:      llmprovidercontroller.NewProviderController(nil, 1),
		model:         llmprovidercontroller.NewModelController(nil, 1),
		modelRole:     llmprovidercontroller.NewModelRoleController(nil, 1),
		internalModel: llmprovidercontroller.NewInternalModelController(nil),
	}, middlewares{requestTimeout: time.Second})
	for _, tc := range []struct{ method, path, code, body string }{
		{http.MethodPost, "/api/v1/providers", "UNAUTHENTICATED", `"code":"UNAUTHENTICATED"`},
		{http.MethodPost, "/healthz", "METHOD_NOT_ALLOWED", `"code":"METHOD_NOT_ALLOWED"`},
		{http.MethodGet, "/missing", "NOT_FOUND", `"code":"NOT_FOUND"`},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		require.Contains(t, rec.Body.String(), tc.body)
		require.Empty(t, rec.Header().Get("Location"))
	}
}
