package tracing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	tracesPath = "/v1/traces"
	logsPath   = "/v1/logs"
)

type Telemetry struct {
	LogHandler slog.Handler
	Shutdown   func(context.Context) error
}

func Setup(ctx context.Context, serviceName, env, otlpEndpoint string) (Telemetry, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if otlpEndpoint == "" {
		return Telemetry{Shutdown: func(context.Context) error { return nil }}, nil
	}
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(serviceName), semconv.DeploymentEnvironment(env))

	traceExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(signalEndpoint(otlpEndpoint, tracesPath)))
	if err != nil {
		return Telemetry{}, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))

	logExp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(signalEndpoint(otlpEndpoint, logsPath)))
	if err != nil {
		return Telemetry{}, fmt.Errorf("log exporter: %w", errors.Join(err, tp.Shutdown(ctx)))
	}
	otel.SetTracerProvider(tp)
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)), sdklog.WithResource(res))

	return Telemetry{
		LogHandler: otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp)),
		Shutdown: func(ctx context.Context) error {
			return errors.Join(tp.Shutdown(ctx), lp.Shutdown(ctx))
		},
	}, nil
}

func signalEndpoint(otlpEndpoint, signalPath string) string {
	return strings.TrimRight(otlpEndpoint, "/") + signalPath
}
