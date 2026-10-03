package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRunInvalidNameLogsOneStructuredError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runMigrationGenerator(context.Background(), []string{"-name", "../escape", "-migrations", "../../migrations"}, &stdout, &stderr)
	require.Equal(t, exitFailure, exitCode)
	require.Empty(t, stdout.String())
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &record))
	require.Equal(t, "VALIDATION_FAILED", record["error_code"])
	require.NotEmpty(t, record["trace_id"])
	require.Contains(t, record["error"], "snake_case")
}

func TestRunPositionalArgumentExplainsFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, exitFailure, runMigrationGenerator(context.Background(), []string{"unexpected"}, &stdout, &stderr))
	require.Contains(t, stderr.String(), "unexpected positional arguments")
}

func TestLogFailureSanitizesDependencyURLs(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", "test", nil)
	require.NoError(t, err)
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(context.Background(), "test")
	defer span.End()
	appErr := apperr.New(apperr.CodeInternalError, apperr.WithCause(fmt.Errorf("open postgres://migrationgen:private@localhost/db")))
	require.Equal(t, exitFailure, recordGenerationFailure(ctx, logger, span, appErr))
	require.NotContains(t, output.String(), "postgres://")
	require.NotContains(t, output.String(), "private")
}

func TestShutdownTracerProviderHasDeadline(t *testing.T) {
	err := shutdownTracerProvider(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(time.Second), deadline, time.Second)
		return nil
	}, time.Second)
	require.NoError(t, err)
}
