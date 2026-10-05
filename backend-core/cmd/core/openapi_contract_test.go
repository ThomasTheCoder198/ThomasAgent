package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIDocumentsAuthenticationAndMutationErrors(t *testing.T) {
	raw, err := os.ReadFile(openAPIPath)
	require.NoError(t, err)
	type operation struct {
		Responses map[string]struct {
			Ref string `yaml:"$ref"`
		} `yaml:"responses"`
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	for path, operations := range doc.Paths {
		if path == "/healthz" || path == "/readyz" {
			continue
		}
		for method, node := range operations {
			if method == "parameters" {
				continue
			}
			var op operation
			require.NoError(t, node.Decode(&op))
			t.Run(method+" "+path, func(t *testing.T) {
				require.Equal(t, "#/components/responses/Unauthorized", op.Responses["401"].Ref)
				if method != "get" && path != "/api/v1/auth/login" {
					require.Equal(t, "#/components/responses/Forbidden", op.Responses["403"].Ref)
				}
			})
		}
	}
	var createModel operation
	node := doc.Paths["/api/v1/models"]["post"]
	require.NoError(t, node.Decode(&createModel))
	for _, status := range []string{"400", "404", "409"} {
		require.Contains(t, createModel.Responses, status)
	}
}
