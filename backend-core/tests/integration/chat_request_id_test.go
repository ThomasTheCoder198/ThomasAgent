package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	chatmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
)

const (
	trailGroup        = "chat-run-workers-test"
	trailConsumer     = "worker-trail"
	trailBlockTimeout = 200 * time.Millisecond
	trailDeliveries   = 4
)

// chatRunsConsumer is the real jobs.Consumer wired to the handler, as core worker wires it.
func (f *workerFixture) chatRunsConsumer(t *testing.T) *jobs.Consumer {
	t.Helper()
	consumer := &jobs.Consumer{Redis: f.redis, Stream: convmodel.StreamChatRuns, GroupName: trailGroup, ConsumerName: trailConsumer,
		MaxDeliveries: trailDeliveries, VisibilityTimeout: f.cfg.JobVisibilityTimeout, BlockTimeout: trailBlockTimeout, BatchSize: 1,
		Concurrency: 1, Log: logger(t, f.logs)}
	require.NoError(t, consumer.EnsureGroup(context.Background()))
	return consumer
}

// TestChatRequestID_OneRunLeavesTheSameIDInEveryPlace: a BeginRun under req_… is traceable through the outbox row,
// the chat.runs stream entry, the worker's logs, every run event entry and a forced error event (amendment W9).
func TestChatRequestID_OneRunLeavesTheSameIDInEveryPlace(t *testing.T) {
	for name, forceError := range map[string]bool{"successful run": false, "forced failure": true} {
		t.Run(name, func(t *testing.T) {
			f := newWorkerFixture(t, nil)
			ctx, requestID := withRequestID(f.ctx)
			run, outboxID, err := f.store.BeginRun(ctx, submitInput(f.conv, f.agent, "u-1", "Giá gói Pro?", "a-1"))
			require.NoError(t, err)

			f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
				require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "500k"}))
				if forceError {
					require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusFailed, string(errors.CodeRagxUnavailable), ""))
					return chatmodel.ErrorEvent(errors.ErrRagxUnavailable, errors.LangVI), nil
				}
				require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "500k"))
				return chatmodel.Finish{}, nil
			}

			// 1. the outbox row
			var outboxRequestID string
			require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT request_id FROM outbox WHERE id = $1`, outboxID).Scan(&outboxRequestID))
			require.Equal(t, requestID, outboxRequestID)

			// 2. the chat.runs stream entry written by the relay
			relay := &jobs.OutboxRelay{Pool: f.pool, Redis: f.redis, BatchSize: 10, Log: logger(t, f.logs)}
			require.NoError(t, relay.PublishOne(context.Background(), outboxID))
			entries, err := f.redis.XRange(context.Background(), convmodel.StreamChatRuns, "-", "+").Result()
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, requestID, entries[0].Values[jobs.StreamFieldRequestID])

			// 3. the consumer restores it into the handler's context, so the worker's logs carry it
			consumer := f.chatRunsConsumer(t)
			require.NoError(t, consumer.PollOnce(context.Background(), f.handler.Handle))
			require.EqualValues(t, 1, f.exec.calls.Load())
			for _, msg := range []string{logRunClaimed, logRunFinished} {
				records := f.logs.find(t, msg)
				require.Len(t, records, 1, msg)
				require.Equal(t, requestID, records[0]["request_id"], msg)
				require.Equal(t, run.ID.String(), records[0]["run_id"], msg)
			}
			handled := f.logs.find(t, "job handled")
			require.Len(t, handled, 1)
			require.Equal(t, requestID, handled[0]["request_id"], "the consumer's own line joins the trail")

			// 4. every event entry of the run stream, including the forced error
			streamed := f.streamEntries(t, run.ID)
			require.Len(t, streamed, 2, "one delta and the terminal event")
			for _, entry := range streamed {
				require.Equal(t, requestID, entry.Values[runstream.FieldRequestID])
			}
			last := f.streamEvents(t, run.ID)[1]
			if forceError {
				require.Equal(t, "RAGX_UNAVAILABLE", last.(chatmodel.Error).Code)
			} else {
				require.Equal(t, chatmodel.Finish{}, last)
			}

			// 5. the stored run, for lastRun
			var storedRequestID string
			require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT request_id FROM runs WHERE id = $1`, run.ID).Scan(&storedRequestID))
			require.Equal(t, requestID, storedRequestID)
			summary, err := f.store.RunStatus(f.ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, requestID, summary.RequestID)
			pending, err := f.redis.XPending(context.Background(), convmodel.StreamChatRuns, trailGroup).Result()
			require.NoError(t, err)
			require.Zero(t, pending.Count, "the job was acked")
		})
	}
}

// entryJSON keeps the helper below readable in failure output.
func entryJSON(entries []redis.XMessage) string {
	raw, _ := json.Marshal(entries)
	return string(raw)
}

func TestChatRequestID_ARunStartedWithoutARequestGetsOneJobIDThroughout(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, outboxID, err := f.store.BeginRun(f.ctx, submitInput(f.conv, f.agent, "u-1", "hi", "a-1"))
	require.NoError(t, err)
	require.NotEmpty(t, run.RequestID)
	relay := &jobs.OutboxRelay{Pool: f.pool, Redis: f.redis, BatchSize: 10, Log: logger(t, f.logs)}
	require.NoError(t, relay.PublishOne(context.Background(), outboxID))
	entries, err := f.redis.XRange(context.Background(), convmodel.StreamChatRuns, "-", "+").Result()
	require.NoError(t, err)
	require.Equal(t, run.RequestID, entries[0].Values[jobs.StreamFieldRequestID], entryJSON(entries))
}
