package main

import (
	"context"
	stderrors "errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx"
)

const dependencyURLPattern = "(?i)(postgres(?:ql)?|redis(?:s)?)://[^\\s`\"<>]+"

type alreadyLoggedError struct{ error }

func (e alreadyLoggedError) Unwrap() error { return e.error }

func sanitizeDependencyError(err error, dsn string) error {
	message := strings.ReplaceAll(err.Error(), dsn, logging.RedactedPlaceholder)
	message = regexp.MustCompile(dependencyURLPattern).ReplaceAllString(message, logging.RedactedPlaceholder)
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

func (a *application) recordDependencyFailure(ctx context.Context, message string, err error, dsn string) error {
	clean := sanitizeDependencyError(err, dsn)
	appErr := errors.ToAppError(err).WithCause(clean)
	a.log.ErrorContext(ctx, message, "error_code", appErr.Code, "error", clean)
	return alreadyLoggedError{appErr}
}

func (a *application) openPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := postgres.Open(ctx, a.cfg.DatabaseURL)
	if err != nil {
		return nil, a.recordDependencyFailure(ctx, "database startup failed", err, a.cfg.DatabaseURL)
	}
	return pool, nil
}

func (a *application) openRedis(ctx context.Context) (*redis.Client, error) {
	redisClient, err := redisx.Open(ctx, a.cfg.RedisURL)
	if err != nil {
		return nil, a.recordDependencyFailure(ctx, "redis startup failed", err, a.cfg.RedisURL)
	}
	return redisClient, nil
}
