package redisx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestReadinessProbe_SuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	client, err := Open(ctx, url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	pinger := ReadinessProbe{Client: client}
	require.NoError(t, pinger.Ping(ctx))
	require.NoError(t, client.Close())
	var appErr *errors.AppError
	require.ErrorAs(t, pinger.Ping(ctx), &appErr)
	require.Equal(t, errors.CodeInternalError, appErr.Code)
}
