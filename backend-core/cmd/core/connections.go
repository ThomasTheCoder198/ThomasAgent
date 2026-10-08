package main

import (
	"context"
	stderrors "errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
)

type alreadyLoggedError struct{ error }

const (
	databaseStartupFailureMessage = "database startup failed"
	redisStartupFailureMessage    = "redis startup failed"
)

func (e alreadyLoggedError) Unwrap() error { return e.error }

func sanitizeDependencyError(err error, dsn string) error {
	message := strings.ReplaceAll(err.Error(), dsn, logging.RedactedPlaceholder)
	message = logging.RedactConnectionURLs(message)
	password := ""
	if parsed, parseErr := url.Parse(dsn); parseErr == nil && parsed.User != nil {
		password, _ = parsed.User.Password()
	}
	if parsed, parseErr := pgxpool.ParseConfig(dsn); parseErr == nil {
		password = parsed.ConnConfig.Password
	}
	if password != "" {
		message = strings.ReplaceAll(message, password, logging.RedactedPlaceholder)
	}
	return stderrors.New(message)
}

func (rt *coreRuntime) recordDependencyFailure(ctx context.Context, message string, err error, dsn string) error {
	clean := sanitizeDependencyError(err, dsn)
	appErr := errors.ToAppError(err).WithCause(clean)
	rt.log.ErrorContext(ctx, message, logging.FieldErrorCode, appErr.Code, logging.FieldError, clean)
	return alreadyLoggedError{appErr}
}

func (rt *coreRuntime) openPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := postgres.Open(ctx, rt.cfg.DatabaseURL)
	if err != nil {
		return nil, rt.recordDependencyFailure(ctx, databaseStartupFailureMessage, err, rt.cfg.DatabaseURL)
	}
	return pool, nil
}

func (rt *coreRuntime) openRedis(ctx context.Context) (*redis.Client, error) {
	redisClient, err := redisx.Open(ctx, rt.cfg.RedisURL)
	if err != nil {
		return nil, rt.recordDependencyFailure(ctx, redisStartupFailureMessage, err, rt.cfg.RedisURL)
	}
	return redisClient, nil
}
