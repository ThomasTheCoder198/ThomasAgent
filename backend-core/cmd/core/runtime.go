package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/telemetry"
)

type application struct {
	cfg         config.Config
	log         *slog.Logger
	telemetry   telemetry.Telemetry
	authService sessionPurger
	metrics     *metrics.Metrics
}

func newApplication(ctx context.Context) (*application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	tel, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.Environment, cfg.LogsOTLPEndpoint)
	if err != nil {
		return nil, err
	}
	log, err := logging.NewLogger(os.Stdout, cfg.LogLevel, cfg.ServiceName, cfg.Environment, tel.LogHandler)
	if err != nil {
		return nil, err
	}
	return &application{cfg: cfg, telemetry: tel, log: log, metrics: metrics.New(cfg.Metrics.HTTPBuckets, cfg.Metrics.LLMBuckets)}, nil
}

func (a *application) shutdownTelemetry() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	return a.telemetry.Shutdown(shutdownCtx)
}
