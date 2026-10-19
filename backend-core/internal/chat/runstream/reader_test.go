package runstream

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx/redistest"
)

const (
	testBlock       = 400 * time.Millisecond
	testStatusCheck = 150 * time.Millisecond
	testSlack       = 5 * time.Second
)

type collector struct{ events []model.Event }

func (c *collector) onEvent(event model.Event) error {
	c.events = append(c.events, event)
	return nil
}

func newTestReader(client *redis.Client, status StatusFunc) *Reader {
	return &Reader{Redis: client, BlockTimeout: testBlock, StatusCheck: testStatusCheck, Status: status}
}

func running(context.Context, uuid.UUID) (convmodel.RunStatus, error) {
	return convmodel.RunStatusRunning, nil
}

func TestFollow_RelaysEventsUntilTheTerminalEvent(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	for _, event := range []model.Event{model.Start{MessageID: "a1", RunID: runID}, model.TextDelta{ID: "t", Delta: "xin chào"}, model.Finish{}} {
		require.NoError(t, writer.Append(context.Background(), runID, deadline, event))
	}
	var got collector
	outcome, err := newTestReader(client, running).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, TerminalRelayed, outcome)
	require.Equal(t, []model.Event{model.Start{MessageID: "a1", RunID: runID}, model.TextDelta{ID: "t", Delta: "xin chào"}, model.Finish{}}, got.events)
}

func TestFollow_AnAbsentStreamMeansNotStartedYet(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	go func() {
		time.Sleep(testBlock + testStatusCheck + 200*time.Millisecond) // longer than one block: the key stays absent that long
		_ = writer.Append(context.Background(), runID, deadline, model.TextDelta{ID: "t", Delta: "late"})
		_ = writer.WriteTerminal(context.Background(), runID, deadline, model.Finish{})
	}()
	var got collector
	outcome, err := newTestReader(client, running).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, TerminalRelayed, outcome)
	require.Equal(t, []model.Event{model.TextDelta{ID: "t", Delta: "late"}, model.Finish{}}, got.events)
}

func TestFollow_TheWindowEndsWithNoTerminal(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(700 * time.Millisecond)
	require.NoError(t, writer.Append(context.Background(), runID, deadline, model.TextDelta{ID: "t", Delta: "partial"}))
	var got collector
	started := time.Now()
	outcome, err := newTestReader(client, running).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, NoTerminal, outcome)
	require.Len(t, got.events, 1, "the partial delta was relayed")
	require.Less(t, time.Since(started), 3*time.Second, "each block is bounded by the time left in the window")
}

func TestFollow_AnExpiredWindowNeverBlocks(t *testing.T) {
	client := redistest.Start(t)
	var got collector
	started := time.Now()
	outcome, err := newTestReader(client, running).Follow(context.Background(), uuid.New(), time.Now().Add(-time.Second), got.onEvent)
	require.NoError(t, err)
	require.Equal(t, NoTerminal, outcome)
	require.Less(t, time.Since(started), testSlack, "BLOCK 0 would wait forever")
}

func TestFollow_RedisFailureFallsBackToNoTerminal(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = dead.Close() })
	var got collector
	outcome, err := newTestReader(dead, running).Follow(context.Background(), uuid.New(), time.Now().Add(time.Minute), got.onEvent)
	require.Error(t, err)
	require.Equal(t, NoTerminal, outcome)
}

func TestFollow_AnEndedRowStopsTheRelayWhenNoTerminalComes(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	require.NoError(t, writer.Append(context.Background(), runID, deadline, model.TextDelta{ID: "t", Delta: "partial"}))
	var got collector
	completed := func(context.Context, uuid.UUID) (convmodel.RunStatus, error) {
		return convmodel.RunStatusCompleted, nil
	}
	outcome, err := newTestReader(client, completed).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, RowEnded, outcome, "no terminal after the row ended: the delta delivery cannot be proven")
	require.Len(t, got.events, 1)
}

func TestFollow_AnEndedRowStillRelaysATerminalEventThatArrivesJustAfter(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	go func() {
		time.Sleep(testStatusCheck + 100*time.Millisecond) // after the row ended, before the final block runs out
		_ = writer.WriteTerminal(context.Background(), runID, deadline, model.Finish{})
	}()
	completed := func(context.Context, uuid.UUID) (convmodel.RunStatus, error) {
		return convmodel.RunStatusCompleted, nil
	}
	var got collector
	outcome, err := newTestReader(client, completed).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, TerminalRelayed, outcome, "the worker writes the terminal event right after the commit")
}

func TestFollow_AStatusErrorKeepsFollowing(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	go func() {
		time.Sleep(testStatusCheck * 3)
		_ = writer.WriteTerminal(context.Background(), runID, deadline, model.Finish{})
	}()
	failing := func(context.Context, uuid.UUID) (convmodel.RunStatus, error) {
		return "", stderrors.New("database down")
	}
	var got collector
	outcome, err := newTestReader(client, failing).Follow(context.Background(), runID, deadline, got.onEvent)
	require.NoError(t, err)
	require.Equal(t, TerminalRelayed, outcome)
}

func TestFollow_StopsWhenTheCallbackFailsOrTheClientLeaves(t *testing.T) {
	client := redistest.Start(t)
	writer := NewWriter(client, testFinishTimeout, testGrace)
	runID := uuid.New()
	deadline := time.Now().Add(time.Minute)
	require.NoError(t, writer.Append(context.Background(), runID, deadline, model.TextDelta{ID: "t", Delta: "x"}))
	boom := stderrors.New("client write failed")
	outcome, err := newTestReader(client, running).Follow(context.Background(), runID, deadline, func(model.Event) error { return boom })
	require.ErrorIs(t, err, boom)
	require.Equal(t, NoTerminal, outcome)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err = newTestReader(client, running).Follow(ctx, runID, deadline, func(model.Event) error { return nil })
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, NoTerminal, outcome)
}
