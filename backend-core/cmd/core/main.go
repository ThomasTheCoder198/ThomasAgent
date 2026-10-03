package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tracing"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const (
	exitOK      = 0
	exitFailure = 1
)

const usage = "usage: core <serve|migrate|relay> [args]"

type app struct {
	cfg      config.Config
	log      *slog.Logger
	tracer   tracing.Providers
	dbTracer trace.Tracer
}

// Deferred cleanup finishes before main translates the result into a process exit code.
func main() { os.Exit(realMain()) }

func realMain() int {
	if len(os.Args) < 2 { //nolint:mnd // argv[0] + subcommand
		fmt.Fprintln(os.Stderr, usage)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "core:", err)
		return exitFailure
	}
	return exitOK
}

func run(ctx context.Context, cmd string, args []string) error {
	a, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.tracer.Shutdown(context.Background()) }()
	switch cmd {
	case "serve":
		return a.serve(ctx)
	case "relay":
		return a.relay(ctx)
	case "migrate":
		return a.migrate(ctx, args)
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
}

func newApp(ctx context.Context) (*app, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	tp, err := tracing.Setup(ctx, cfg.ServiceName, cfg.Env, cfg.OTLPEndpoint)
	if err != nil {
		return nil, err
	}
	return &app{cfg: cfg, tracer: tp, dbTracer: otel.Tracer(cfg.ServiceName), log: logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName, tp.LogHandler)}, nil
}

func (a *app) serve(ctx context.Context) error {
	pool, err := a.openDatabase(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, err := a.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	r := httpx.NewRouter(httpx.SlogErrorLogger(a.log), httpx.Tracing(a.cfg.ServiceName), httpx.AccessLog(a.log))
	httpx.MountHealth(r, a.readinessDeps(pool, rdb))
	srv := &http.Server{Addr: a.cfg.HTTPAddr, Handler: r, ReadHeaderTimeout: a.cfg.ReadHeaderTimeout}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	a.log.Info("core listening", "addr", a.cfg.HTTPAddr)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func (a *app) readinessDeps(pool *pgxpool.Pool, rdb *redis.Client) map[string]httpx.Pinger {
	return map[string]httpx.Pinger{"postgres": pool, "redis": redisx.Pinger{Client: rdb}}
}

func (a *app) migrate(ctx context.Context, args []string) error {
	ctx, span := a.dbTracer.Start(ctx, "postgres.migrate", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	dir := postgres.Up
	if len(args) > 0 {
		dir = postgres.Direction(args[0])
	}
	if err := postgres.Migrate(ctx, a.cfg.DatabaseURL, migrations.FS, dir); err != nil {
		span.SetStatus(codes.Error, "migration failed")
		a.log.ErrorContext(ctx, "migration failed", "error_code", apperr.CodeInternalError)
		return err
	}
	a.log.InfoContext(ctx, "migrations applied", "direction", string(dir))
	return nil
}

func (a *app) openDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	ctx, span := a.dbTracer.Start(ctx, "postgres.open", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	pool, err := postgres.Open(ctx, a.cfg.DatabaseURL)
	if err != nil {
		span.SetStatus(codes.Error, "database startup failed")
		a.log.ErrorContext(ctx, "database startup failed", "error_code", apperr.CodeInternalError)
		return nil, err
	}
	return pool, nil
}

func (a *app) openRedis(ctx context.Context) (*redis.Client, error) {
	ctx, span := a.dbTracer.Start(ctx, "redis.startup")
	defer span.End()
	rdb, err := redisx.Open(ctx, a.cfg.RedisURL)
	if err != nil {
		span.SetStatus(codes.Error, "redis startup failed")
		a.log.ErrorContext(ctx, "redis startup failed", "error_code", apperr.From(err).Code)
		return nil, err
	}
	return rdb, nil
}

func (a *app) relay(ctx context.Context) error {
	pool, err := a.openDatabase(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, err := a.openRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	r := &jobs.Relay{Pool: pool, Redis: rdb, BatchSize: a.cfg.Relay.BatchSize, PollInterval: a.cfg.Relay.PollInterval, Log: a.log}
	a.log.InfoContext(ctx, "outbox relay started")
	return r.Run(ctx)
}
