package runstream

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx/redistest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	testFinishTimeout = 10 * time.Second
	testGrace         = 2 * time.Minute
)

func TestWriter_AppendsEventsWithRequestIDAndNoTrimming(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	id := requestid.NewRequest()
	ctx := requestid.NewContext(context.Background(), id)
	deadline := time.Now().Add(5 * time.Minute)

	const events = 1500 // above go-redis' default approximate trim thresholds: nothing may be trimmed
	for range events {
		require.NoError(t, writer.Append(ctx, runID, deadline, model.TextDelta{ID: "t1", Delta: "x"}))
	}
	require.NoError(t, writer.WriteTerminal(ctx, runID, deadline, model.Finish{}))

	length, err := client.XLen(ctx, StreamKey(runID)).Result()
	require.NoError(t, err)
	require.EqualValues(t, events+1, length)
	entries, err := client.XRange(ctx, StreamKey(runID), "-", "+").Result()
	require.NoError(t, err)
	for _, entry := range entries {
		require.Equal(t, id, entry.Values[FieldRequestID], "each event entry carries the request id")
		_, err := DecodeEvent([]byte(entry.Values[FieldEvent].(string)))
		require.NoError(t, err)
	}
}

func TestWriter_SetsTheExpiryOnTheFirstWriteOnly(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Minute).Truncate(time.Second)

	require.NoError(t, writer.Append(ctx, runID, deadline, model.TextStart{ID: "t1"}))
	want := deadline.Add(testFinishTimeout).Add(testGrace)
	expireTime := func() time.Time {
		seconds, err := client.ExpireTime(ctx, StreamKey(runID)).Result()
		require.NoError(t, err)
		return time.Unix(int64(seconds/time.Second), 0)
	}
	require.Equal(t, want.Unix(), expireTime().Unix(), "EXPIREAT deadline + finish timeout + grace")

	require.NoError(t, writer.Append(ctx, runID, deadline.Add(time.Hour), model.TextEnd{ID: "t1"}))
	require.Equal(t, want.Unix(), expireTime().Unix(), "a later write does not move it")
}

func TestWriter_WriteTerminalRefusesANonTerminalEvent(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	require.Error(t, writer.WriteTerminal(context.Background(), runID, time.Now().Add(time.Minute), model.TextDelta{}))
	length, err := client.XLen(context.Background(), StreamKey(runID)).Result()
	require.NoError(t, err)
	require.Zero(t, length)
}
