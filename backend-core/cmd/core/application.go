package main

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tracing"
)

type application struct {
	cfg           config.Config
	log           *slog.Logger
	telemetry     tracing.Telemetry
	startupTracer trace.Tracer
}

func newApplication(ctx context.Context) (*application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	tp, err := tracing.Setup(ctx, cfg.ServiceName, cfg.Environment, cfg.OTLPEndpoint)
	if err != nil {
		return nil, err
	}
	log, err := logging.NewLogger(os.Stdout, cfg.LogLevel, cfg.ServiceName, cfg.Environment, tp.LogHandler)
	if err != nil {
		return nil, err
	}
	return &application{cfg: cfg, telemetry: tp, startupTracer: otel.Tracer(cfg.ServiceName), log: log}, nil
}

func (a *application) shutdownTelemetry() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	return a.telemetry.Shutdown(shutdownCtx)
}
