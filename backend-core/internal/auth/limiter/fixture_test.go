package limiter

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const ownerEmail = "thomas@example.com"

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(context.Background(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	redisURL, err := container.ConnectionString(context.Background())
	require.NoError(t, err)
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

// scriptExpiryFailureHook fails every Lua script call to prove a failed script leaves no key.
type scriptExpiryFailureHook struct{ failure error }

func (h scriptExpiryFailureHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h scriptExpiryFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h scriptExpiryFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "evalsha" || cmd.Name() == "eval" {
			return h.failure
		}
		return next(ctx, cmd)
	}
}
