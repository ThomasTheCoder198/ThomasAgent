package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/schemadiff"
)

const (
	exitOK               = 0
	exitFailure          = 1
	serviceName          = "migrationgen"
	defaultSchemaPath    = "schema.sql"
	defaultMigrationsDir = "migrations"
	logLevel             = "info"
	flagName             = "name"
	flagSchema           = "schema"
	flagMigrations       = "migrations"
	dependencyURLPattern = "(?i)(postgres(?:ql)?|redis(?:s)?)://[^\\s`\"<>]+"
)

func main() { os.Exit(runProcess()) }

func shutdownTracerProvider(shutdown func(context.Context) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return shutdown(ctx)
}

func runProcess() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runMigrationGenerator(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

func runMigrationGenerator(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	logger, err := logging.New(stderr, logLevel, serviceName, nil)
	if err != nil {
		return exitFailure
	}
	settings, configErr := config.LoadSchema()
	provider := sdktrace.NewTracerProvider()
	// With no exporter, an invalid config's zero timeout still releases in-process resources immediately.
	defer func() { _ = shutdownTracerProvider(provider.Shutdown, settings.CleanupTimeout) }()
	ctx, span := provider.Tracer(serviceName).Start(ctx, "migrationgen.run")
	defer span.End()
	request, err := parseGenerationRequest(args)
	if err != nil {
		return recordGenerationFailure(ctx, logger, span, err)
	}
	if configErr != nil {
		return recordGenerationFailure(ctx, logger, span, configErr)
	}
	result, err := schemadiff.New(settings, time.Now, provider.Tracer(serviceName)).Generate(ctx, request)
	if err != nil {
		return recordGenerationFailure(ctx, logger, span, err)
	}
	if result.Path == "" {
		_, err = fmt.Fprintln(stdout, "Schema and migration history match; no migration created.")
	} else {
		_, err = fmt.Fprintln(stdout, result.Path)
	}
	if err != nil {
		return recordGenerationFailure(ctx, logger, span, err)
	}
	logger.InfoContext(ctx, "migration generation completed", "migration", result.Path)
	return exitOK
}

func parseGenerationRequest(args []string) (schemadiff.Request, error) {
	flags := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var request schemadiff.Request
	flags.StringVar(&request.MigrationName, flagName, "", "snake_case migration name")
	flags.StringVar(&request.SchemaPath, flagSchema, defaultSchemaPath, "desired PostgreSQL schema file")
	flags.StringVar(&request.MigrationsDir, flagMigrations, defaultMigrationsDir, "Goose migration history directory")
	if err := flags.Parse(args); err != nil {
		return schemadiff.Request{}, errors.ErrValidationFailed.WithCause(err)
	}
	if len(flags.Args()) != 0 {
		return schemadiff.Request{}, errors.ErrValidationFailed.WithMessage("unexpected positional arguments")
	}
	return request, nil
}

func recordGenerationFailure(ctx context.Context, logger *slog.Logger, span trace.Span, err error) int {
	appErr := errors.From(err)
	urlPattern := regexp.MustCompile(dependencyURLPattern)
	safeError := urlPattern.ReplaceAllString(err.Error(), logging.Redacted)
	safeMessage := urlPattern.ReplaceAllString(appErr.LocalizedMessage(errors.LangEN), logging.Redacted)
	span.RecordError(fmt.Errorf("%s", safeError))
	logger.ErrorContext(ctx, "migration generation failed", "error_code", appErr.Code, "error", safeError, "message", safeMessage)
	return exitFailure
}
