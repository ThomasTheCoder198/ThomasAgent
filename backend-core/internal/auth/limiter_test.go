package auth

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// expireFailureHook fails expiry after the real Redis increment succeeds.
type expireFailureHook struct{ failure error }

func (h expireFailureHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h expireFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "expire" {
			return h.failure
		}
		return next(ctx, cmd)
	}
}
func (h expireFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestLimiterExpiryFailureIsReturned(t *testing.T) {
	client := startRedis(t)
	failure := stderrors.New("test Redis expiry failure")
	client.AddHook(expireFailureHook{failure})
	err := NewLimiter(client, 2, time.Minute).Allow(t.Context(), "expiry-test")
	require.ErrorIs(t, err, failure)
	require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
	remaining, readErr := client.Exists(t.Context(), limiterKeyPrefix+"expiry-test").Result()
	require.NoError(t, readErr)
	require.Zero(t, remaining, "expiry failure must not leave an immortal limiter key")
}
