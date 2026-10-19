package redisx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx/redistest"
)

const (
	probeStream = "chat.runs"
	probeGroup  = "workers"
)

func TestGroupProbe_ReadyOnlyWhenTheGroupExists(t *testing.T) {
	client := redistest.Start(t)
	probe := GroupProbe{Client: client, Stream: probeStream, Group: probeGroup}
	ctx := context.Background()
	require.Error(t, probe.Ping(ctx), "no stream yet")

	require.NoError(t, client.XGroupCreateMkStream(ctx, probeStream, "someone-else", "0").Err())
	require.Error(t, probe.Ping(ctx), "the stream exists but not this group")

	require.NoError(t, client.XGroupCreateMkStream(ctx, probeStream, probeGroup, "0").Err())
	require.NoError(t, probe.Ping(ctx))
}
