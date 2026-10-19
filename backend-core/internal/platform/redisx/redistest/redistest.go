package redistest

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const image = "redis:8.10.2"

// Start returns a client for a disposable Redis; the container is terminated by test cleanup.
func Start(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(t.Context(), image)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	address, err := container.ConnectionString(t.Context())
	require.NoError(t, err)
	options, err := redis.ParseURL(address)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	return client
}
