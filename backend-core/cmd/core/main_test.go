package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tracing"
)

func TestApplication_DatabaseFailuresRecordDiagnosticsOnce(t *testing.T) {
	for _, operation := range []string{"migrate", "open"} {
		t.Run(operation, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			var output bytes.Buffer
			log, logErr := logging.NewLogger(&output, "info", "core-test", "test", nil)
			require.NoError(t, logErr)
			a := &application{
				cfg:           config.Config{DatabaseURL: "postgres://nobody:secret-password@127.0.0.1:1/none?connect_timeout=1"},
				log:           log,
				startupTracer: provider.Tracer("core-test"),
			}
			var err error
			if operation == "migrate" {
				err = a.applyMigrations(context.Background(), nil)
			} else {
				_, err = a.openPostgres(context.Background())
			}
			require.Error(t, err)
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, "core.postgres."+operation, spans[0].Name())
			require.Equal(t, codes.Error, spans[0].Status().Code)
			var record map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &record))
			require.Equal(t, "ERROR", record["level"])
			require.Equal(t, spans[0].SpanContext().TraceID().String(), record["trace_id"])
			require.Equal(t, spans[0].SpanContext().SpanID().String(), record["span_id"])
			require.Equal(t, "INTERNAL_ERROR", record["error_code"])
			require.NotEmpty(t, record["error"])
			require.Contains(t, record["error"], "127.0.0.1")
			require.Len(t, spans[0].Events(), 1)
			for _, attribute := range spans[0].Events()[0].Attributes {
				require.NotContains(t, attribute.Value.AsString(), "secret-password")
				require.NotContains(t, attribute.Value.AsString(), "postgres://")
			}
			require.NotContains(t, output.String(), "secret-password")
			require.NotContains(t, output.String(), "postgres://")
		})
	}
}

func TestRealMain_Helper(t *testing.T) {
	if os.Getenv("CORE_TEST_REAL_MAIN") != "1" {
		return
	}
	os.Args = []string{"core"}
	if command := os.Getenv("CORE_TEST_COMMAND"); command != "" {
		os.Args = append(os.Args, command)
	}
	os.Exit(runProcess())
}

func TestProcess_ReportsUsageBeforeLoadingConfiguration(t *testing.T) {
	for _, command := range []string{"", "unknown"} {
		t.Run(command, func(t *testing.T) {
			t.Setenv("CORE_TEST_REAL_MAIN", "1")
			t.Setenv("CORE_TEST_COMMAND", command)
			t.Setenv("CORE_SHUTDOWN_TIMEOUT", "invalid-duration")
			process := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRealMain_Helper$")
			output, err := process.CombinedOutput()
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, exitFailure, exitErr.ExitCode())
			require.Contains(t, string(output), usageMessage)
			require.NotContains(t, string(output), "invalid-duration")
			require.NotContains(t, string(output), "error_code")
			if command != "" {
				require.Contains(t, string(output), `unknown command "`+command+`"`)
			}
		})
	}
}

func TestRealMain_LogsDatabaseFailureOnce(t *testing.T) {
	for _, command := range []string{"migrate", "serve", outboxRelayCommand} {
		t.Run(command, func(t *testing.T) {
			setIdentityEnv(t)
			t.Setenv("CORE_TEST_REAL_MAIN", "1")
			t.Setenv("CORE_TEST_COMMAND", command)
			t.Setenv("CORE_DATABASE_URL", "postgres://nobody:secret-password@127.0.0.1:1/none?connect_timeout=1")
			t.Setenv("CORE_REDIS_URL", "redis://127.0.0.1:1")
			t.Setenv("CORE_OTLP_ENDPOINT", "")
			process := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRealMain_Helper$")
			output, err := process.CombinedOutput()
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, exitFailure, exitErr.ExitCode())
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			require.Len(t, lines, 1, string(output))
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
			require.NotEmpty(t, record["error"])
			require.NotContains(t, string(output), "postgres://")
			require.NotContains(t, string(output), "secret-password")
		})
	}
}

func TestSanitizeDependencyError_RemovesCredentials(t *testing.T) {
	for _, fixture := range []struct{ dsn, password string }{
		{"postgres://nobody:secret-password@localhost/none", "secret-password"},
		{"postgres://nobody:secret%2Fpassword@localhost/none", "secret/password"},
		{"host=localhost user=nobody password=secret-password dbname=none", "secret-password"},
	} {
		t.Run(fixture.dsn, func(t *testing.T) {
			err := errors.New("connection failed: " + fixture.dsn + "; " + fixture.password + "; dial refused")
			clean := sanitizeDependencyError(err, fixture.dsn)
			require.Contains(t, clean.Error(), "dial refused")
			require.NotContains(t, clean.Error(), fixture.dsn)
			require.NotContains(t, clean.Error(), "postgres://")
			require.NotContains(t, clean.Error(), fixture.password)
		})
	}
}

func TestApplication_TelemetryShutdownHasDeadline(t *testing.T) {
	a := &application{
		cfg: config.Config{ShutdownTimeout: 20 * time.Millisecond},
		telemetry: tracing.Telemetry{Shutdown: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(20*time.Millisecond), deadline, 10*time.Millisecond)
			<-ctx.Done()
			return ctx.Err()
		}},
	}
	require.ErrorIs(t, a.shutdownTelemetry(), context.DeadlineExceeded)
}

type cancelOnListeningWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelOnListeningWriter) Write(line []byte) (int, error) {
	n, err := w.Buffer.Write(line)
	if bytes.Contains(line, []byte("core listening")) {
		w.cancel()
	}
	return n, err
}

func TestServe_ListeningLogCarriesTraceContext(t *testing.T) {
	ctx := t.Context()
	database, err := tcpostgres.Run(ctx, "postgres:18.6-alpine",
		tcpostgres.WithDatabase("core"), tcpostgres.WithUsername("core"), tcpostgres.WithPassword("core"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Terminate(context.Background())) })
	databaseURL, err := database.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	redis, err := tcredis.Run(ctx, "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, redis.Terminate(context.Background())) })
	redisURL, err := redis.ConnectionString(ctx)
	require.NoError(t, err)
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	ctx, span := provider.Tracer("core-test").Start(ctx, "serve-test")
	defer span.End()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &cancelOnListeningWriter{cancel: cancel}
	log, err := logging.NewLogger(output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	a := &application{
		cfg: config.Config{DatabaseURL: databaseURL, RedisURL: redisURL, HTTPAddr: "127.0.0.1:0", ShutdownTimeout: time.Second},
		log: log, startupTracer: provider.Tracer("core-test"),
	}
	require.NoError(t, a.serveHTTP(ctx))
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])
}

func TestHealthcheck_DoesNotOpenDependencies(t *testing.T) {
	setIdentityEnv(t)
	t.Setenv("CORE_DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/none")
	t.Setenv("CORE_REDIS_URL", "redis://127.0.0.1:1")
	t.Setenv("CORE_OTLP_ENDPOINT", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/healthz", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("CORE_HTTP_ADDR", strings.TrimPrefix(server.URL, "http://127.0.0.1"))
	require.NoError(t, executeCommand(context.Background(), "healthcheck", nil))
}

func TestHealthcheck_RejectsUnhealthyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	require.ErrorContains(t, checkHTTPHealth(context.Background(), strings.TrimPrefix(server.URL, "http://127.0.0.1")), "503")
}

func TestSpanNames_UseCoreServicePrefix(t *testing.T) {
	require.Equal(t, []string{"core.postgres.open", "core.redis.startup", "core.postgres.migrate"}, []string{postgresOpenSpan, redisStartupSpan, postgresMigrateSpan})
}

// testVaultMasterKey is the base64 of 32 ASCII bytes.
const testVaultMasterKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

// setIdentityEnv sets the variables that config requires on top of the database and Redis URLs.
func setIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CORE_VAULT_MASTER_KEY", testVaultMasterKey)
}
