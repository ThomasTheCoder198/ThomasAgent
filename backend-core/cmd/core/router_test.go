package main

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const openAPIPath = "../../../contracts/openapi/core.v1.yaml"

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
	r := newRouter(routerDeps{log: slog.New(slog.NewTextHandler(io.Discard, nil)), serviceName: "test"})
	documented := documentedRoutes(t)
	var missing []string
	implemented := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + strings.TrimSuffix(route, "/")
		if route == "/" {
			key = method + " /"
		}
		if !documented[key] {
			missing = append(missing, key)
		}
		implemented[key] = true
		return nil
	})
	require.NoError(t, err)
	sort.Strings(missing)
	require.Empty(t, missing, "add these routes to contracts/openapi/core.v1.yaml, then run task gen")
	var unimplemented []string
	for key := range documented {
		if !implemented[key] {
			unimplemented = append(unimplemented, key)
		}
	}
	sort.Strings(unimplemented)
	require.Empty(t, unimplemented, "remove stale paths from contracts/openapi/core.v1.yaml")
}
