package integration

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	chatmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	relayBlock       = 300 * time.Millisecond
	relayStatusCheck = 100 * time.Millisecond
)

func (f *workerFixture) reader(client *redis.Client) *runstream.Reader {
	return &runstream.Reader{Redis: client, BlockTimeout: relayBlock, StatusCheck: relayStatusCheck,
		Status: func(ctx context.Context, runID uuid.UUID) (convmodel.RunStatus, error) {
			run, err := f.store.RunStatus(ctx, runID)
			return run.Status, err
		}}
}

// relay follows the run the way the chat controller will and returns what the client would have received last.
func (f *workerFixture) relay(t *testing.T, client *redis.Client, run convmodel.Run, window time.Time) (runstream.Outcome, []chatmodel.Event, chatmodel.Event) {
	t.Helper()
	var received []chatmodel.Event
	outcome, _ := f.reader(client).Follow(f.ctx, run.ID, window, func(event chatmodel.Event) error {
		received = append(received, event)
		return nil
	})
	row, err := f.store.RunStatus(f.ctx, run.ID)
	require.NoError(t, err)
	return outcome, received, runstream.RelayEnd(outcome, row, time.Now(), f.cfg.FinishTimeout)
}

func (f *workerFixture) window(run convmodel.Run) time.Time {
	return run.DeadlineAt.Add(f.cfg.FinishTimeout)
}

func TestRunStream_ATerminalEventRelayedEndsWithFinish(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "xin chào"}))
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "xin chào"))
		return chatmodel.Finish{}, nil
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	outcome, received, end := f.relay(t, f.redis, run, f.window(run))
	require.Equal(t, runstream.TerminalRelayed, outcome)
	require.Equal(t, []chatmodel.Event{chatmodel.TextDelta{ID: "t1", Delta: "xin chào"}, chatmodel.Finish{}}, received)
	require.Equal(t, chatmodel.Finish{}, end)
}

// TestRunStream_MissingTerminalAfterPartialDeltasIsStreamIncomplete: the row finished but the client was never told
// (the terminal event did not arrive), so it must not be shown finish (amendment W4).
func TestRunStream_MissingTerminalAfterPartialDeltasIsStreamIncomplete(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, _, _, _ := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	require.NoError(t, f.writer.Append(workerCtx(run.RequestID), run.ID, run.DeadlineAt, chatmodel.TextDelta{ID: "t1", Delta: "một phần"}))
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{ID: "a-1", Role: convmodel.RoleAssistant, Parts: []byte(`[{"type":"text","text":"cả câu"}]`)}))

	outcome, received, end := f.relay(t, f.redis, run, f.window(run))
	require.Equal(t, runstream.RowEnded, outcome)
	require.Len(t, received, 1, "the partial delta was relayed")
	require.Equal(t, "RUN_STREAM_INCOMPLETE", end.(chatmodel.Error).Code, "the client refetches the stored answer")
}

func TestRunStream_EndedRowStopsContinuousNonterminalTraffic(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, _, _, _ := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{}))
	write := func() error {
		return f.writer.Append(workerCtx(run.RequestID), run.ID, run.DeadlineAt, chatmodel.TextDelta{ID: "t1", Delta: "still arriving"})
	}
	require.NoError(t, write())
	reader := f.reader(f.redis)
	reader.StatusCheck = time.Nanosecond
	readCtx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	count := 0
	errTooManyEvents := stderrors.New("ended row was ignored during continuous traffic")
	outcome, err := reader.Follow(readCtx, run.ID, f.window(run), func(chatmodel.Event) error {
		count++
		if count >= 100 {
			return errTooManyEvents
		}
		return write()
	})
	require.NoError(t, err)
	require.Equal(t, runstream.RowEnded, outcome)
	require.Less(t, count, 100, "continuous nonterminal traffic must not postpone the row check")
}

func TestRunStream_AnAbsentStreamThenEventsAppearIsRelayed(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "muộn"}))
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "muộn"))
		return chatmodel.Finish{}, nil
	}
	go func() {
		time.Sleep(relayBlock + relayStatusCheck + 250*time.Millisecond) // the worker has not written anything yet
		_ = f.handler.Handle(ctx, msg)
	}()
	outcome, received, end := f.relay(t, f.redis, run, f.window(run))
	require.Equal(t, runstream.TerminalRelayed, outcome, "an absent key is not started yet, never ended")
	require.Equal(t, []chatmodel.Event{chatmodel.TextDelta{ID: "t1", Delta: "muộn"}, chatmodel.Finish{}}, received)
	require.Equal(t, chatmodel.Finish{}, end)
}

// TestRunStream_RedisFailureFallsBackToTheRow: Redis is unreachable for the relay after FinishRun committed.
func TestRunStream_RedisFailureFallsBackToTheRow(t *testing.T) {
	cases := map[string]struct {
		status convmodel.RunStatus
		code   string
		want   string
	}{
		"completed": {convmodel.RunStatusCompleted, "", "RUN_STREAM_INCOMPLETE"},
		"cancelled": {convmodel.RunStatusCancelled, "", "RUN_STREAM_INCOMPLETE"},
		"failed":    {convmodel.RunStatusFailed, string(errors.CodeRunOutputTooLarge), "RUN_OUTPUT_TOO_LARGE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newWorkerFixture(t, nil)
			run, _, _, _ := f.begin(t, "1")
			lease := claim(t, f.store, f.ctx, run)
			run.Status, run.ErrorCode = tc.status, tc.code
			require.NoError(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{}))
			dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
			t.Cleanup(func() { _ = dead.Close() })

			outcome, received, end := f.relay(t, dead, run, f.window(run))
			require.Equal(t, runstream.NoTerminal, outcome)
			require.Empty(t, received)
			require.Equal(t, tc.want, end.(chatmodel.Error).Code)
		})
	}
}

func TestRunStream_StillRunningPastDeadlinePlusFinishIsInterrupted(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, _, _, _ := f.begin(t, "1")
	claim(t, f.store, f.ctx, run)
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 minute' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	run.DeadlineAt = time.Now().Add(-time.Minute)

	started := time.Now()
	outcome, received, end := f.relay(t, f.redis, run, f.window(run))
	require.Equal(t, runstream.NoTerminal, outcome)
	require.Empty(t, received)
	require.Equal(t, "RUN_INTERRUPTED", end.(chatmodel.Error).Code)
	require.Less(t, time.Since(started), testSignalWait, "an over window never blocks")
}
