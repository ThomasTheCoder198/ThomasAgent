package auth

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const limiterKeyPrefix = "thomas:auth:login:"

type Limiter struct {
	rdb    *redis.Client
	max    int
	window time.Duration
}

func NewLimiter(rdb *redis.Client, max int, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, max: max, window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) error {
	k := limiterKeyPrefix + key
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		return errors.ErrInternalError.WithCause(fmt.Errorf("login limiter increment: %w", err))
	}
	if n == 1 {
		if err := l.setExpiry(ctx, k); err != nil {
			return err
		}
	}
	if n > int64(l.max) {
		return errors.New(errors.CodeRateLimited)
	}
	return nil
}

func (l *Limiter) setExpiry(ctx context.Context, key string) error {
	err := l.rdb.Expire(ctx, key, l.window).Err()
	if err == nil {
		return nil
	}
	// An unexpired counter would lock this client out after Redis recovers.
	if cleanupErr := l.rdb.Del(ctx, key).Err(); cleanupErr != nil {
		err = stderrors.Join(err, fmt.Errorf("login limiter cleanup: %w", cleanupErr))
	}
	return errors.ErrInternalError.WithCause(fmt.Errorf("login limiter expiry: %w", err))
}
