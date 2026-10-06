package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Telemetry is core's log export: core has no traces (spec 2026-10-06 §4.1).
type Telemetry struct {
	LogHandler slog.Handler
	Shutdown   func(context.Context) error
}

// Setup exports logs over OTLP/HTTP to logsEndpoint, used as the complete URL (for example
// http://victorialogs:9428/insert/opentelemetry/v1/logs). An empty endpoint disables export.
func Setup(ctx context.Context, serviceName, env, logsEndpoint string) (Telemetry, error) {
	if logsEndpoint == "" {
		return Telemetry{Shutdown: func(context.Context) error { return nil }}, nil
	}
	exporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(logsEndpoint))
	if err != nil {
		return Telemetry{}, fmt.Errorf("log exporter: %w", err)
	}
	// Resource attributes become VictoriaLogs stream fields, so only stable low-cardinality values go here.
	res := resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName), semconv.DeploymentEnvironment(env))
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)), sdklog.WithResource(res))
	return Telemetry{
		LogHandler: otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(provider)),
		Shutdown:   provider.Shutdown,
	}, nil
}
