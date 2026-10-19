package integration

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/control"
	chatmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/redisx/redistest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	testSignalWait   = 10 * time.Second
	testTraceParent  = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	testTraceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
	logRunClaimed    = "chat run claimed"
	logRunFinished   = "chat run finished"
	logRunInterrupt  = "chat run interrupted"
	logRunDropped    = "chat run dropped"
	testOrgID        = "org-a"
	assistantTextFmt = `[{"type":"text","text":"%s"}]`
)

// logBuffer is a goroutine-safe slog sink whose lines are parsed as JSON records.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) find(t *testing.T, msg string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == msg {
			out = append(out, record)
		}
	}
	return out
}

type queueRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (q *queueRecorder) ObserveChatRunQueue(waited time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.waits = append(q.waits, waited)
}

// executorFunc is a fake executor; calls counts how often the handler ran it.
type executorFunc struct {
	calls atomic.Int32
	run   func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error)
}

type failOnceRunStore struct {
	convservice.ChatStore
	failed atomic.Bool
}

func (s *failOnceRunStore) FailRun(ctx context.Context, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error) {
	if !s.failed.Swap(true) {
		return false, stderrors.New("temporary database outage")
	}
	return s.ChatStore.FailRun(ctx, runID, code, onlyIfExpired)
}

func (e *executorFunc) Execute(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
	e.calls.Add(1)
	return e.run(ctx, in)
}

type workerFixture struct {
	*leaseFixture
	redis   *redis.Client
	bus     *control.Bus
	writer  *runstream.Writer
	cfg     config.ChatConfig
	logs    *logBuffer
	metrics *queueRecorder
	exec    *executorFunc
	handler *worker.Handler
}

func baseChatConfig() config.ChatConfig {
	return config.ChatConfig{
		RunDeadline: time.Minute, FinishTimeout: 5 * time.Second, JobVisibilityTimeout: 2 * time.Minute, RunLease: testRunLease,
		RunHeartbeatTimeout: 5 * time.Second, RunStreamMaxEvents: 100, RunStreamMaxBytes: 1 << 20, RunStreamGrace: 2 * time.Minute,
		RelayStatusCheck: 5 * time.Second,
	}
}

func newWorkerFixture(t *testing.T, mutate func(*config.ChatConfig)) *workerFixture {
	t.Helper()
	cfg := baseChatConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	f := &workerFixture{leaseFixture: newLeaseFixture(t), redis: redistest.Start(t), cfg: cfg, logs: &logBuffer{}, metrics: &queueRecorder{}, exec: &executorFunc{}}
	f.bus = control.NewBus(f.redis)
	f.writer = runstream.NewWriter(f.redis, cfg.FinishTimeout, cfg.RunStreamGrace)
	f.handler = &worker.Handler{Store: f.store, Control: f.bus, Stream: f.writer, Executor: f.exec, Cfg: cfg,
		Log: logger(t, f.logs), Metrics: f.metrics, Owner: "worker-under-test"}
	return f
}

// workerCtx is the context jobs.Consumer builds for a delivered job: tenant and request id, no user.
func workerCtx(requestID string) context.Context {
	return requestid.NewContext(tenant.WithID(context.Background(), testOrgID), requestID)
}

func (f *workerFixture) message(run convmodel.Run, requestID, traceParent string) jobs.Message {
	payload, err := json.Marshal(convmodel.ChatRunJob{RunID: run.ID, ConversationID: run.ConversationID, UserID: f.user})
	if err != nil {
		panic(err)
	}
	return jobs.Message{TenantID: testOrgID, Stream: convmodel.StreamChatRuns, Payload: payload, RequestID: requestID, TraceParent: traceParent}
}

// begin starts a run under a fresh request id and returns the handler context and message of its job.
func (f *workerFixture) begin(t *testing.T, tag string) (convmodel.Run, context.Context, jobs.Message, string) {
	t.Helper()
	ctx, requestID := withRequestID(f.ctx)
	run, _ := beginSubmit(t, f.store, ctx, f.conv, f.agent, "u-"+tag, "a-"+tag)
	return run, workerCtx(requestID), f.message(run, requestID, ""), requestID
}

func (f *workerFixture) streamEntries(t *testing.T, runID uuid.UUID) []redis.XMessage {
	t.Helper()
	entries, err := f.redis.XRange(context.Background(), runstream.StreamKey(runID), "-", "+").Result()
	require.NoError(t, err)
	return entries
}

func (f *workerFixture) streamEvents(t *testing.T, runID uuid.UUID) []chatmodel.Event {
	t.Helper()
	var events []chatmodel.Event
	for _, entry := range f.streamEntries(t, runID) {
		event, err := runstream.DecodeEvent([]byte(entry.Values[runstream.FieldEvent].(string)))
		require.NoError(t, err)
		events = append(events, event)
	}
	return events
}

// finishRun is what a real executor does last: the fenced FinishRun with the run's final state.
func (f *workerFixture) finishRun(ctx context.Context, in worker.ExecuteInput, status convmodel.RunStatus, code, text string) error {
	run := in.Run
	run.Status, run.ErrorCode = status, code
	assistant := convmodel.Message{}
	if text != "" {
		assistant = convmodel.Message{ID: in.Run.AssistantMessageID, Role: convmodel.RoleAssistant, Parts: json.RawMessage(strings.Replace(assistantTextFmt, "%s", text, 1))}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), testSignalWait)
	defer cancel()
	return f.store.FinishRun(finishCtx, in.Lease, run, assistant)
}

func TestChatWorker_NormalRunRelaysEventsAndTheTerminalEvent(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, requestID := f.begin(t, "1")
	var executorUser uuid.UUID
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		user, _ := authmiddleware.UserFromContext(ctx)
		executorUser = user.ID
		require.NoError(t, in.Emit(chatmodel.Start{MessageID: in.Run.AssistantMessageID, RunID: in.Run.ID}))
		for _, delta := range []string{"500k ", "một ", "tháng"} {
			require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: delta}))
		}
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "500k một tháng"))
		return chatmodel.Finish{}, nil
	}

	require.NoError(t, f.handler.Handle(ctx, msg))

	require.EqualValues(t, 1, f.exec.calls.Load())
	require.Equal(t, f.user, executorUser, "the handler puts the job's user into the executor context")
	events := f.streamEvents(t, run.ID)
	require.Equal(t, []chatmodel.Event{
		chatmodel.Start{MessageID: "a-1", RunID: run.ID}, chatmodel.TextDelta{ID: "t1", Delta: "500k "},
		chatmodel.TextDelta{ID: "t1", Delta: "một "}, chatmodel.TextDelta{ID: "t1", Delta: "tháng"}, chatmodel.Finish{},
	}, events, "the events in order, then the terminal event after FinishRun")
	for _, entry := range f.streamEntries(t, run.ID) {
		require.Equal(t, requestID, entry.Values[runstream.FieldRequestID])
	}
	status, code, token, until := f.runRow(t, run.ID)
	require.Equal(t, "completed", status)
	require.Empty(t, code)
	require.Nil(t, token, "the lease is cleared")
	require.Nil(t, until)
	require.Equal(t, 1, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE id = 'a-1' AND role = 'assistant'`))
	require.Len(t, f.metrics.waits, 1, "the queue wait was observed once")

	claimed := f.logs.find(t, logRunClaimed)
	require.Len(t, claimed, 1)
	require.Equal(t, run.ID.String(), claimed[0]["run_id"])
	require.Equal(t, requestID, claimed[0]["request_id"])
	finished := f.logs.find(t, logRunFinished)
	require.Len(t, finished, 1)
	require.Equal(t, run.ID.String(), finished[0]["run_id"])
	require.Equal(t, requestID, finished[0]["request_id"])
	require.Contains(t, finished[0], "duration_ms")
}

func TestChatWorker_StopPublishedBeforeTheWorkerSubscribedIsSeen(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	require.NoError(t, f.bus.Stop(context.Background(), run.ID, time.Now().Add(time.Minute)))
	sawStop := false
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		select {
		case <-in.Control.Stopped():
			sawStop = true
		default:
		}
		require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "dở dang"}))
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCancelled, "", "dở dang"))
		return chatmodel.Finish{}, nil
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	require.True(t, sawStop, "subscribe-then-check finds the key a stop left behind")
	status, _, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "cancelled", status)
}

func TestChatWorker_StopBetweenStepsReachesTheExecutor(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	stepDone := make(chan struct{})
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "bước 1"}))
		close(stepDone)
		select {
		case <-in.Control.Stopped():
		case <-time.After(testSignalWait):
			t.Error("the stop never reached the executor")
		}
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCancelled, "", "bước 1"))
		return chatmodel.Finish{}, nil
	}
	done := make(chan error, 1)
	go func() { done <- f.handler.Handle(ctx, msg) }()
	select {
	case <-stepDone:
	case <-time.After(testSignalWait):
		t.Fatal("the executor never started")
	}
	require.NoError(t, f.bus.Stop(context.Background(), run.ID, time.Now().Add(time.Minute)))
	require.NoError(t, <-done)
	status, _, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "cancelled", status)
}

func TestChatWorker_HeartbeatFailureCancelsTheExecutorAndTheHandlerWritesNothing(t *testing.T) {
	f := newWorkerFixture(t, func(cfg *config.ChatConfig) {
		cfg.RunLease, cfg.RunHeartbeatTimeout = 900*time.Millisecond, 200*time.Millisecond // heartbeat every 300ms
	})
	run, ctx, msg, _ := f.begin(t, "1")
	started := make(chan struct{})
	var cause error
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		close(started)
		select {
		case <-ctx.Done():
			cause = context.Cause(ctx)
		case <-time.After(testSignalWait):
			t.Error("the run context was never cancelled")
		}
		return nil, ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- f.handler.Handle(ctx, msg) }()
	<-started
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_token = gen_random_uuid() WHERE id = $1`, run.ID)
	require.NoError(t, err, "another executor took the lease")
	require.NoError(t, <-done)

	require.ErrorIs(t, cause, errors.ErrRunLeaseLost, "a refused renewal cancels the run at once")
	status, _, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "running", status, "the row now belongs to whoever holds the lease")
	require.Empty(t, f.streamEntries(t, run.ID), "no terminal event from a worker that lost the lease")
	require.Len(t, f.logs.find(t, logRunInterrupt), 1)
}

func TestChatWorker_OverTheEventCapFailsTheRunWithRunOutputTooLarge(t *testing.T) {
	for name, mutate := range map[string]func(*config.ChatConfig){
		"events": func(cfg *config.ChatConfig) { cfg.RunStreamMaxEvents = 3 },
		"bytes":  func(cfg *config.ChatConfig) { cfg.RunStreamMaxBytes = 400 },
	} {
		t.Run(name, func(t *testing.T) {
			f := newWorkerFixture(t, mutate)
			run, ctx, msg, _ := f.begin(t, "1")
			var emitErr error
			f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
				for range 20 {
					if emitErr = in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "một đoạn khá dài để vượt giới hạn byte"}); emitErr != nil {
						break
					}
				}
				code := worker.CauseCode(ctx)
				require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusFailed, code, ""))
				return chatmodel.Error{Code: code}, nil
			}
			require.NoError(t, f.handler.Handle(ctx, msg))

			require.ErrorIs(t, emitErr, errors.ErrRunOutputTooLarge)
			status, code, _, _ := f.runRow(t, run.ID)
			require.Equal(t, "failed", status)
			require.Equal(t, "RUN_OUTPUT_TOO_LARGE", code)
			events := f.streamEvents(t, run.ID)
			require.Equal(t, chatmodel.Error{Code: "RUN_OUTPUT_TOO_LARGE"}, events[len(events)-1])
			require.Less(t, len(events), 20, "writing stopped at the cap")
		})
	}
}

func TestChatWorker_SuccessTerminalAfterCapCancellationFailsTheRun(t *testing.T) {
	f := newWorkerFixture(t, func(cfg *config.ChatConfig) { cfg.RunStreamMaxEvents = 1 })
	run, ctx, msg, _ := f.begin(t, "1")
	f.exec.run = func(_ context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		require.NoError(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "part"}))
		require.ErrorIs(t, in.Emit(chatmodel.TextDelta{ID: "t1", Delta: "overflow"}), errors.ErrRunOutputTooLarge)
		return chatmodel.Finish{}, nil // a faulty executor ignored Emit's cancellation
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	status, code, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, string(errors.CodeRunOutputTooLarge), code)
	events := f.streamEvents(t, run.ID)
	require.Equal(t, []chatmodel.Event{chatmodel.TextDelta{ID: "t1", Delta: "part"}, f.terminalError(errors.CodeRunOutputTooLarge)}, events)
}

func TestChatWorker_DuplicateDeliveryWhileRunningIsDroppedAndTheExecutorRunsOnce(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	started, release := make(chan struct{}), make(chan struct{})
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		close(started)
		<-release
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "xong"))
		return chatmodel.Finish{}, nil
	}
	first := make(chan error, 1)
	go func() { first <- f.handler.Handle(ctx, msg) }()
	<-started

	duplicate := make(chan error, 1)
	go func() { duplicate <- f.handler.Handle(ctx, msg) }()
	select {
	case err := <-duplicate:
		require.NoError(t, err, "a duplicate is acked and dropped")
	case <-time.After(testSignalWait):
		t.Fatal("the duplicate delivery blocked")
	}
	close(release)
	require.NoError(t, <-first)
	require.EqualValues(t, 1, f.exec.calls.Load(), "one executor per run")
	require.Equal(t, 1, countRows(t, f.pool, `SELECT count(*) FROM runs WHERE id = $1 AND status = 'completed'`, run.ID))
	require.Len(t, f.logs.find(t, logRunDropped), 1)
}

func TestChatWorker_ADeadExecutorIsInterruptedOnceAndNeverRerun(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, requestID := f.begin(t, "1")
	claim(t, f.store, f.ctx, run) // the first worker claimed it, then died
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	f.exec.run = func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) {
		t.Error("an interrupted run must never be executed again")
		return nil, nil
	}

	require.NoError(t, f.handler.Handle(ctx, msg))
	require.NoError(t, f.handler.Handle(ctx, msg), "a later duplicate finds the run ended")

	require.Zero(t, f.exec.calls.Load())
	status, code, token, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_INTERRUPTED", code)
	require.Nil(t, token)
	entries := f.streamEntries(t, run.ID)
	require.Len(t, entries, 1, "the terminal event is written once")
	require.Equal(t, requestID, entries[0].Values[runstream.FieldRequestID])
	require.Equal(t, "RUN_INTERRUPTED", f.streamEvents(t, run.ID)[0].(chatmodel.Error).Code)
}

func TestChatWorker_AQueuedRunPastItsDeadlineFailsWithoutRunning(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	f.exec.run = func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) {
		t.Error("a run past its deadline must not execute")
		return nil, nil
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	status, code, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", code)
	require.Equal(t, []chatmodel.Event{f.terminalError(errors.CodeRunDeadlineExceeded)}, f.streamEvents(t, run.ID))
}

func (f *workerFixture) terminalError(code errors.Code) chatmodel.Event {
	return chatmodel.ErrorEvent(errors.New(code), errors.LangVI)
}

func TestChatWorker_ARunThatIsNotRunningIsAckedAndDropped(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{}))
	f.exec.run = func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) {
		t.Error("a finished run must not execute")
		return nil, nil
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	require.Empty(t, f.streamEntries(t, run.ID))
}

func TestChatWorker_AnExecutorThatReturnsAnErrorFailsTheRunOnce(t *testing.T) {
	for name, exec := range map[string]func(context.Context, worker.ExecuteInput) (chatmodel.Event, error){
		"error": func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) {
			return nil, errors.ErrRagxUnavailable
		},
		"panic": func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newWorkerFixture(t, nil)
			run, ctx, msg, _ := f.begin(t, "1")
			f.exec.run = exec
			require.NoError(t, f.handler.Handle(ctx, msg), "never retried: the run is decided")
			status, code, token, _ := f.runRow(t, run.ID)
			require.Equal(t, "failed", status)
			require.Nil(t, token)
			want := errors.CodeRagxUnavailable
			if name == "panic" {
				want = errors.CodeInternalError
			}
			require.Equal(t, string(want), code)
			require.Equal(t, []chatmodel.Event{f.terminalError(want)}, f.streamEvents(t, run.ID))
		})
	}
}

func TestChatWorker_TransientFailRunFailureKeepsTheJobPendingUntilLeaseExpiry(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	f.handler.Store = &failOnceRunStore{ChatStore: f.store}
	f.exec.run = func(context.Context, worker.ExecuteInput) (chatmodel.Event, error) {
		return nil, errors.ErrRagxUnavailable
	}

	require.Error(t, f.handler.Handle(ctx, msg), "a failed settlement must leave the Redis delivery pending")
	status, _, token, _ := f.runRow(t, run.ID)
	require.Equal(t, "running", status)
	require.NotNil(t, token)
	require.Empty(t, f.streamEntries(t, run.ID))

	msg.Deliveries = 2
	require.Error(t, f.handler.Handle(ctx, msg), "a redelivery while the old lease is live must stay pending")
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	msg.Deliveries = 3
	require.NoError(t, f.handler.Handle(ctx, msg), "after the lease expires the pending delivery closes the run")
	status, code, token, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, string(errors.CodeRunInterrupted), code)
	require.Nil(t, token)
	require.Equal(t, []chatmodel.Event{f.terminalError(errors.CodeRunInterrupted)}, f.streamEvents(t, run.ID))
	require.EqualValues(t, 1, f.exec.calls.Load(), "the run is not executed twice")
}

func TestChatWorker_ALeaseLostDuringFinishWritesNoTerminalEvent(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, msg, _ := f.begin(t, "1")
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, in.Run.ID)
		require.NoError(t, err)
		return nil, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "muộn")
	}
	require.NoError(t, f.handler.Handle(ctx, msg))
	require.Empty(t, f.streamEntries(t, run.ID))
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE role = 'assistant'`), "the fenced finish stored nothing")
	flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
	require.NoError(t, err)
	require.True(t, flipped, "the expiry owns the row; one terminal event will come from it")
}

func TestChatWorker_ARunDeadlineCancelsTheExecutorWithItsCode(t *testing.T) {
	f := newWorkerFixture(t, nil)
	outerCtx, _ := withRequestID(f.ctx)
	msgInput := submitInput(f.conv, f.agent, "u-1", "chậm", "a-1")
	msgInput.Deadline = time.Now().Add(1500 * time.Millisecond)
	run, _, err := f.store.BeginRun(outerCtx, msgInput)
	require.NoError(t, err)
	var causeCode string
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		select {
		case <-ctx.Done():
		case <-time.After(testSignalWait):
			t.Error("the deadline never cancelled the run")
		}
		causeCode = worker.CauseCode(ctx)
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusFailed, causeCode, ""))
		return f.terminalError(errors.CodeRunDeadlineExceeded), nil
	}
	require.NoError(t, f.handler.Handle(workerCtx(run.RequestID), f.message(run, run.RequestID, "")))
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", causeCode)
	status, code, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", code)
}

func TestChatWorker_TheRunSpanContinuesTheRequestTrace(t *testing.T) {
	f := newWorkerFixture(t, nil)
	run, ctx, _, requestID := f.begin(t, "1")
	var traceID string
	f.exec.run = func(ctx context.Context, in worker.ExecuteInput) (chatmodel.Event, error) {
		traceID = trace.SpanContextFromContext(ctx).TraceID().String()
		require.NoError(t, f.finishRun(ctx, in, convmodel.RunStatusCompleted, "", "ok"))
		return chatmodel.Finish{}, nil
	}
	require.NoError(t, f.handler.Handle(ctx, f.message(run, requestID, testTraceParent)))
	require.Equal(t, testTraceID, traceID, "traceparent from the outbox row becomes the parent of the run")
}

func TestChatWorker_AMalformedJobIsAnErrorSoItIsRetriedThenDeadLettered(t *testing.T) {
	f := newWorkerFixture(t, nil)
	for _, payload := range []string{`not json`, `{}`, `{"runId":"` + uuid.NewString() + `"}`} {
		err := f.handler.Handle(workerCtx(requestid.NewRequest()), jobs.Message{Payload: []byte(payload)})
		require.ErrorIs(t, err, errors.ErrChatRunJobInvalid, payload)
	}
	require.Zero(t, f.exec.calls.Load())
}
