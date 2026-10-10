package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	appcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/controller"
	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	orgcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/controller"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const openAPIPath = "../../../contracts/openapi/core.v1.yaml"

func testControllers() ControllerContainer {
	return ControllerContainer{
		AuthController:     authcontroller.NewAuthController(authcontroller.AuthControllerParams{}),
		ProviderController: llmcontroller.NewProviderController(nil, nil, 1),
		LLMModelController: llmcontroller.NewLLMModelController(nil, 1),
		RoleController:     llmcontroller.NewRoleController(nil, 1),

		OrganizationController: orgcontroller.NewOrganizationController(nil, 1),
		AppProxyController:     appcontroller.NewAppProxyController(nil, 1),
	}
}

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
	registerRoutes(r, testControllers(), middlewares{requestTimeout: time.Second})
	documented := documentedRoutes(t)
	implemented := map[string]bool{}
	var missing []string
	for _, route := range r.Routes() {
		key := route.Method + " " + httpx.ContractRoute(route.Path)
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
	registerRoutes(r, testControllers(), middlewares{requestTimeout: time.Second})
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

func TestRoutes_AppProxyServesOnlySixMethodsBehindTheSession(t *testing.T) {
	r := httpx.NewRouter(nil)
	registerRoutes(r, testControllers(), middlewares{requestTimeout: time.Second})
	path := "/api/v1/orgs/3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21/apps/ragx/knowledge-bases"
	for _, method := range []string{http.MethodOptions, http.MethodTrace} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, method)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code, "the session check runs before RequireOrg and the proxy")
}
