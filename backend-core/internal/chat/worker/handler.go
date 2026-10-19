package worker

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/control"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const (
	tracerName      = "thomas/chat/worker"
	spanName        = "chat.run"
	traceParentName = "traceparent"

	logClaimed     = "chat run claimed"
	logFinished    = "chat run finished"
	logInterrupted = "chat run interrupted"
	logDropped     = "chat run dropped"
	logTerminal    = "chat run terminal event not written"

	fieldOwner   = "owner"
	fieldReason  = "reason"
	fieldOutcome = "outcome"
	fieldQueueMS = "queue_ms"

	reasonLeaseLost          = "lease_lost"
	reasonLeaseExpired       = "lease_expired"
	reasonQueuedPastDeadline = "queued_past_deadline"
	reasonExecutorFailed     = "executor_failed"
	reasonControlUnavailable = "control_unavailable"

	outcomeCompleted = "completed"
	outcomeError     = "error"

	// terminalLang words the message of an error the worker writes; the UI stream carries only the code.
	terminalLang = errors.LangVI
)

// Store is the slice of the chat store the handler needs (declared here by its consumer, rule 2).
type Store interface {
	ClaimRun(ctx context.Context, runID uuid.UUID, owner string, lease time.Duration) (convmodel.Run, convmodel.Lease, convmodel.ClaimResult, error)
	RenewLease(ctx context.Context, lease convmodel.Lease, d time.Duration) error
	FailRun(ctx context.Context, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error)
}

// Control subscribes a worker to a run's stop channel.
type Control interface {
	Subscribe(ctx context.Context, runID uuid.UUID) (control.Subscription, error)
}

// Stream writes a run's events for the API relay.
type Stream interface {
	Append(ctx context.Context, runID uuid.UUID, deadline time.Time, event model.Event) error
	WriteTerminal(ctx context.Context, runID uuid.UUID, deadline time.Time, event model.Event) error
}

// Metrics records how long a run waited for a worker (chat_run_queue_seconds). Nil disables it.
type Metrics interface {
	ObserveChatRunQueue(waited time.Duration)
}

// Handler consumes the chat.runs stream: it claims a run under a lease, runs the Executor, relays its events and
// writes the terminal event once the run's end is stored (amendment W5). A run executes at most once, and every
// settled outcome of a claimed run is a normal return (the job is acked): a failed run is never re-executed.
type Handler struct {
	Store    Store
	Control  Control
	Stream   Stream
	Executor Executor
	Cfg      config.ChatConfig
	Log      *slog.Logger
	Metrics  Metrics
	// Owner names this worker process in lease_owner and in logs; it must be unique per process.
	Owner string
}

// Handle is a jobs.Handler. It returns an error only when nothing was decided yet (a bad payload, the database
// unreachable before the claim), so the job is retried or dead-lettered.
func (h *Handler) Handle(ctx context.Context, msg jobs.Message) error {
	job, err := decodeJob(msg.Payload)
	if err != nil {
		return err
	}
	ctx = authmiddleware.WithUser(ctx, authmodel.User{ID: job.UserID})
	ctx, span := startSpan(ctx, msg.TraceParent)
	defer span.End()

	run, lease, result, err := h.Store.ClaimRun(ctx, job.RunID, h.Owner, h.Cfg.RunLease)
	if err != nil {
		return err
	}
	switch result {
	case convmodel.ClaimResultClaimed:
		return h.execute(ctx, run, lease)
	case convmodel.ClaimResultExpired:
		return h.endUnclaimed(ctx, run, errors.CodeRunInterrupted, true, reasonLeaseExpired)
	case convmodel.ClaimResultQueuedPastDeadline:
		return h.endUnclaimed(ctx, run, errors.CodeRunDeadlineExceeded, false, reasonQueuedPastDeadline)
	case convmodel.ClaimResultHeldElsewhere:
		// A retry must stay pending until the old lease lapses; the first delivery may still be running normally.
		if msg.Deliveries > 1 {
			return errors.ErrRunLeaseHeld
		}
		h.Log.InfoContext(ctx, logDropped, logging.FieldRunID, job.RunID, fieldReason, string(result))
		return nil
	default:
		h.Log.InfoContext(ctx, logDropped, logging.FieldRunID, job.RunID, fieldReason, string(result))
		return nil
	}
}

func decodeJob(payload []byte) (convmodel.ChatRunJob, error) {
	var job convmodel.ChatRunJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return job, fmt.Errorf("%w: %w", errors.ErrChatRunJobInvalid, err)
	}
	if job.RunID == uuid.Nil || job.UserID == uuid.Nil {
		return job, errors.ErrChatRunJobInvalid
	}
	return job, nil
}

// startSpan makes the run's span a child of the HTTP request's (amendment W9). Without an installed tracer the span
// is non-recording but still carries the trace id, so outgoing calls keep joining the trace.
func startSpan(ctx context.Context, traceParent string) (context.Context, trace.Span) {
	if traceParent != "" {
		ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{traceParentName: traceParent})
	}
	return otel.Tracer(tracerName).Start(ctx, spanName)
}

// endUnclaimed finishes a run nobody can execute any more. Only the process that really changed the row writes the
// terminal event, so it is written once however many deliveries arrive.
func (h *Handler) endUnclaimed(ctx context.Context, run convmodel.Run, code errors.Code, onlyIfExpired bool, reason string) error {
	flipped, err := h.Store.FailRun(ctx, run.ID, string(code), onlyIfExpired)
	if err != nil {
		return err
	}
	if !flipped {
		h.Log.InfoContext(ctx, logDropped, logging.FieldRunID, run.ID, fieldReason, "already_ended")
		return nil
	}
	h.Log.WarnContext(ctx, logInterrupted, logging.FieldRunID, run.ID, fieldReason, reason, logging.FieldErrorCode, string(code))
	h.writeTerminal(ctx, run, model.ErrorEvent(errors.New(code), terminalLang))
	return nil
}

func (h *Handler) execute(ctx context.Context, run convmodel.Run, lease convmodel.Lease) error {
	started := time.Now()
	if h.Metrics != nil {
		h.Metrics.ObserveChatRunQueue(run.QueuedFor)
	}
	h.Log.InfoContext(ctx, logClaimed, logging.FieldRunID, run.ID, fieldOwner, h.Owner, fieldQueueMS, run.QueuedFor.Milliseconds())

	// Subscribe first, then the bus checks the stop key: a Stop sent at any moment reaches the executor.
	subscription, err := h.Control.Subscribe(ctx, run.ID)
	if err != nil {
		return h.abandon(ctx, run, started, reasonControlUnavailable, err)
	}
	defer func() { _ = subscription.Close() }()

	cancelCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	runCtx, stopDeadline := context.WithDeadlineCause(cancelCtx, run.DeadlineAt, errors.ErrRunDeadlineExceeded)
	defer stopDeadline()

	stopBeating := make(chan struct{})
	var beating sync.WaitGroup
	beating.Add(1)
	go func() {
		defer beating.Done()
		h.heartbeat(ctx, lease, cancel, stopBeating)
	}()
	sink := &eventSink{handler: h, run: run, cancel: cancel, runCtx: runCtx}
	terminal, execErr := h.safeExecute(runCtx, ExecuteInput{Run: run, Lease: lease, Emit: sink.emit, Control: subscription})
	close(stopBeating)
	beating.Wait()
	return h.conclude(ctx, run, started, terminal, execErr, context.Cause(runCtx))
}

// safeExecute keeps a panicking executor from taking the whole worker down with every other run in it.
func (h *Handler) safeExecute(ctx context.Context, in ExecuteInput) (terminal model.Event, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			terminal, err = nil, fmt.Errorf("%w: %v", errors.ErrRunExecutorPanic, recovered)
		}
	}()
	return h.Executor.Execute(ctx, in)
}

// heartbeat renews the lease three times per lease period. A failed or refused renewal cancels the run at once, so
// provider and tool calls stop, and ends the heartbeat: the lease is gone.
func (h *Handler) heartbeat(ctx context.Context, lease convmodel.Lease, cancel context.CancelCauseFunc, stop <-chan struct{}) {
	ticker := time.NewTicker(h.Cfg.RunLease / config.HeartbeatDivisor)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := h.renew(ctx, lease); err != nil {
				h.Log.WarnContext(ctx, "chat run heartbeat failed", logging.FieldRunID, lease.RunID, logging.FieldError, err.Error())
				cancel(errors.ErrRunLeaseLost)
				return
			}
		}
	}
}

// renew runs under WithoutCancel so a cancelled run still renews while it finishes, bounded by its own timeout.
func (h *Handler) renew(ctx context.Context, lease convmodel.Lease) error {
	renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.Cfg.RunHeartbeatTimeout)
	defer cancel()
	return h.Store.RenewLease(renewCtx, lease, h.Cfg.RunLease)
}

// conclude settles a run after its executor returned.
func (h *Handler) conclude(ctx context.Context, run convmodel.Run, started time.Time, terminal model.Event, execErr, cause error) error {
	_, successTerminal := terminal.(model.Finish)
	switch {
	case stderrors.Is(execErr, errors.ErrRunLeaseLost) || stderrors.Is(cause, errors.ErrRunLeaseLost):
		// Whoever took the row over writes its terminal event; this worker may not touch the run any more.
		h.Log.WarnContext(ctx, logInterrupted, logging.FieldRunID, run.ID, fieldReason, reasonLeaseLost, logging.FieldDurationMS, time.Since(started).Milliseconds())
		return nil
	case stderrors.Is(cause, errors.ErrRunOutputTooLarge) && execErr == nil && successTerminal:
		// A success terminal cannot override Emit's cap cancellation while the row is still running.
		return h.failRun(ctx, run, started, errors.CodeRunOutputTooLarge, errors.ErrRunOutputTooLarge)
	case execErr == nil && terminal != nil:
		h.writeTerminal(ctx, run, terminal)
		h.logFinished(ctx, run, started, terminal)
		return nil
	default:
		if execErr == nil {
			execErr = errors.ErrRunExecutorNoTerminal
		}
		return h.failRun(ctx, run, started, failureCode(execErr, cause), execErr)
	}
}

// failureCode picks the catalog code of a run its executor could not end: a cap or deadline that cancelled it,
// else the error's own code.
func failureCode(execErr, cause error) errors.Code {
	switch {
	case stderrors.Is(cause, errors.ErrRunOutputTooLarge):
		return errors.CodeRunOutputTooLarge
	case stderrors.Is(cause, errors.ErrRunDeadlineExceeded):
		return errors.CodeRunDeadlineExceeded
	default:
		return errors.CodeOf(execErr)
	}
}

// failRun is the safety net for an executor that returned without ending the run. It never re-executes: the run
// becomes failed, unless the executor already ended it, and the terminal event follows the change.
func (h *Handler) failRun(ctx context.Context, run convmodel.Run, started time.Time, code errors.Code, cause error) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.Cfg.FinishTimeout)
	defer cancel()
	flipped, err := h.Store.FailRun(finishCtx, run.ID, string(code), false)
	if err != nil {
		return err
	}
	h.Log.WarnContext(ctx, logInterrupted, logging.FieldRunID, run.ID, fieldReason, reasonExecutorFailed, logging.FieldErrorCode, string(code),
		logging.FieldError, cause.Error(), logging.FieldDurationMS, time.Since(started).Milliseconds())
	if flipped {
		h.writeTerminal(ctx, run, model.ErrorEvent(errors.New(code), terminalLang))
	}
	return nil
}

// abandon ends a run this worker claimed but cannot execute (the control channel is unreachable).
func (h *Handler) abandon(ctx context.Context, run convmodel.Run, started time.Time, reason string, cause error) error {
	h.Log.ErrorContext(ctx, "chat run cannot start", logging.FieldRunID, run.ID, fieldReason, reason, logging.FieldError, cause.Error())
	return h.failRun(ctx, run, started, errors.CodeRunInterrupted, cause)
}

// writeTerminal writes the closing event after the row is final; a failure here is logged once and left to the
// relay's database fallback, because the run itself is already decided.
func (h *Handler) writeTerminal(ctx context.Context, run convmodel.Run, event model.Event) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.Cfg.FinishTimeout)
	defer cancel()
	if err := h.Stream.WriteTerminal(finishCtx, run.ID, run.DeadlineAt, event); err != nil {
		h.Log.ErrorContext(ctx, logTerminal, logging.FieldRunID, run.ID, logging.FieldErrorCode, string(errors.CodeOf(err)))
	}
}

func (h *Handler) logFinished(ctx context.Context, run convmodel.Run, started time.Time, terminal model.Event) {
	elapsed := time.Since(started).Milliseconds()
	if failure, isError := terminal.(model.Error); isError {
		h.Log.WarnContext(ctx, logFinished, logging.FieldRunID, run.ID, fieldOutcome, outcomeError, logging.FieldErrorCode, failure.Code, logging.FieldDurationMS, elapsed)
		return
	}
	h.Log.InfoContext(ctx, logFinished, logging.FieldRunID, run.ID, fieldOutcome, outcomeCompleted, logging.FieldDurationMS, elapsed)
}

// eventSink is the Emit of one run: it writes to the stream and enforces the caps (amendment W4).
type eventSink struct {
	handler *Handler
	run     convmodel.Run
	cancel  context.CancelCauseFunc
	runCtx  context.Context

	mu     sync.Mutex
	events int
	bytes  int64
}

func (s *eventSink) emit(event model.Event) error {
	if runstream.IsTerminal(event) {
		return fmt.Errorf("%w: the handler writes the terminal event", errors.ErrRunEventInvalid)
	}
	raw, err := runstream.EncodeEvent(event)
	if err != nil {
		return err
	}
	if s.overCap(int64(len(raw))) {
		s.cancel(errors.ErrRunOutputTooLarge)
		return errors.ErrRunOutputTooLarge
	}
	// A cancelled run still closes its partial answer: the write keeps its own bounded context.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(s.runCtx), s.handler.Cfg.FinishTimeout)
	defer cancel()
	return s.handler.Stream.Append(writeCtx, s.run.ID, s.run.DeadlineAt, event)
}

// overCap counts the event and reports whether it breaks CORE_CHAT_RUN_STREAM_MAX_EVENTS or _MAX_BYTES.
func (s *eventSink) overCap(size int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events++
	s.bytes += size
	return s.events > s.handler.Cfg.RunStreamMaxEvents || s.bytes > s.handler.Cfg.RunStreamMaxBytes
}
