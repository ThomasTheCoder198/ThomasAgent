package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestServe_InvalidVaultKeyFailsBeforeDependencies(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	app := &application{
		cfg: config.Config{
			Vault:       config.VaultConfig{KeyID: "v1", MasterKey: "bad"},
			DatabaseURL: "postgres://test:never-log-this@127.0.0.1:1/test?connect_timeout=1",
		},
		log: logger, startupTracer: provider.Tracer("core-test"),
	}
	err = app.serveHTTP(t.Context())
	require.ErrorContains(t, err, "CORE_VAULT_MASTER_KEY")
	require.NotContains(t, err.Error(), "never-log-this")
	require.NotContains(t, output.String(), "core listening")
	require.Empty(t, output.String(), "vault validation must precede dependency connections")
}
