package service

import (
	"context"
	stderrors "errors"
	"strconv"
	"sync"
	"time"

	"github.com/firebase/genkit/go/ai"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/uistream"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const (
	stepKindModel   = convmodel.StepKindModel
	stepKindTool    = convmodel.StepKindTool
	stepKindRewrite = convmodel.StepKindRewrite
	outcomeSuccess  = convmodel.StepOutcomeSuccess
	outcomeError    = convmodel.StepOutcomeError
	outcomeSkipped  = convmodel.StepOutcomeSkipped
	idPrefixReason  = "r"
	idPrefixText    = "t"

	logRunFinished   = "chat run loop finished"
	logRunSaveError  = "chat run save failed"
	logRunCloseError = "chat run close after save failure failed"
	logStepError     = "chat run step record failed"
	logStopCheck     = "chat run stop check failed"
	fieldStatus      = "status"
	fieldSteps       = "steps"

	// errorCodeCancelled is the error_code of a step or tool part that a Stop or an interruption cut short.
	errorCodeCancelled = "CANCELLED"
)

// run is one agent run: it owns the message builder and the run-wide state. Tool goroutines emit concurrently, so emit
// and the counters are guarded by mu.
type run struct {
	svc *RunServiceImpl
	in  worker.ExecuteInput
	// parent is the handler's run context: it ends at the run deadline, a stream cap or a lost lease, and CauseCode and
	// context.Cause tell which. ctx is the executor's own child: it also ends on a user Stop and on a failed Emit.
	parent context.Context
	ctx    context.Context
	cancel context.CancelCauseFunc
	lang   errors.Lang

	conv     convmodel.Conversation
	agent    agentmodel.Agent
	resolved llmmodel.ResolvedModel
	chat     genkitx.ChatModel
	history  convmodel.History
	builder  *uistream.MessageBuilder
	sources  *sourceRegistry

	mu        sync.Mutex
	open      map[string]string // tool calls started and not finished: call id → tool
	kbDropped bool
	ids       int
	step      int
	usage     model.RunUsage
	ttft      time.Duration
	started   time.Time
	// emitErr is the first error Emit returned. After it nothing more is written to the stream, and the run can no
	// longer end completed (ruling F2bW2-6): the browser may have missed events the stored message contains.
	emitErr error

	fastOnce     sync.Once
	fast         *genkitx.ChatModel
	fastResolved llmmodel.ResolvedModel
}

func (s *RunServiceImpl) newRun(parent context.Context, in worker.ExecuteInput) *run {
	ctx, cancel := context.WithCancelCause(parent)
	lang := errors.LangVI
	if in.Run.Locale == convmodel.LocaleEN {
		lang = errors.LangEN
	}
	return &run{
		svc: s, in: in, parent: parent, ctx: ctx, cancel: cancel, lang: lang,
		builder: uistream.NewMessageBuilder(), sources: newSourceRegistry(), open: map[string]string{},
		started: s.deps.Now(), usage: model.RunUsage{RunID: in.Run.ID.String()},
	}
}

// emit records the event for the stored message, then writes it to the run stream. After the first stream failure the
// run is cancelled, since nobody can see what it would still produce.
func (r *run) emit(event model.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builder.Apply(event)
	switch ev := event.(type) {
	case model.ToolInputAvailable:
		r.open[ev.CallID] = ev.Tool
	case model.ToolOutputAvailable:
		delete(r.open, ev.CallID)
	case model.ToolOutputError:
		delete(r.open, ev.CallID)
	}
	if r.emitErr != nil {
		return
	}
	if err := r.in.Emit(event); err != nil {
		r.emitErr = err
		r.cancel(err)
	}
}

func (r *run) nextID(prefix string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids++
	return prefix + strconv.Itoa(r.ids)
}

func (r *run) status(key model.StatusKey) {
	r.emit(model.Status{Label: model.StatusLabel(key, r.lang)})
}

func (r *run) execute() (model.Event, error) {
	defer r.cancel(nil)
	stopWatching := r.watchStop()
	defer stopWatching()
	r.emit(model.Start{MessageID: r.in.Run.AssistantMessageID, RunID: r.in.Run.ID})
	err := r.prepare()
	if err == nil {
		err = r.loop(r.initialMessages())
	}
	return r.finish(err)
}

// prepare loads what the loop needs: the conversation with its history (the user message BeginRun stored is the last
// one), the Agent, and the chat model (B2-8). A failure here ends the run before it produced anything.
func (r *run) prepare() error {
	history, err := r.svc.deps.Store.LoadHistory(r.ctx, r.in.Run.ConversationID, r.svc.deps.Config.HistoryTurns)
	if err != nil {
		return err
	}
	agent, err := r.svc.deps.Agents.Get(r.ctx, history.Conversation.AgentID)
	if err != nil {
		return err
	}
	resolved, chat, err := r.svc.chatModel(r.ctx, agent, r.in.Run.RequestedModelID)
	if err != nil {
		return err
	}
	r.history, r.conv, r.agent, r.resolved, r.chat = history, history.Conversation, agent, resolved, chat
	return nil
}

// watchStop turns a published Stop into a cancellation of the run's own context, so a model stream or a tool call that
// is in flight aborts at once. The handler does not cancel its context on Stop, because the run must still save.
func (r *run) watchStop() func() {
	if r.in.Control == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-r.in.Control.Stopped():
			r.cancel(errors.ErrRunStopped)
		case <-r.ctx.Done():
		case <-done:
		}
	}()
	return func() { close(done) }
}

// stopRequested checks the stop key; the loop calls it before each model step and each tool batch (PW-2b4). A check
// that fails is logged and ignored: the published Stop still reaches watchStop.
func (r *run) stopRequested() bool {
	if r.in.Control == nil {
		return false
	}
	stopped, err := r.in.Control.Check(r.ctx)
	if err != nil {
		r.svc.deps.Log.WarnContext(r.ctx, logStopCheck, logging.FieldRunID, r.in.Run.ID, logging.FieldError, err.Error())
		return false
	}
	if stopped {
		r.cancel(errors.ErrRunStopped)
	}
	return stopped
}

// leaseLost reports that this executor no longer owns the run: the handler's heartbeat was refused, or a fenced write
// found another token. Nothing may be written then; whoever took the row over writes its end.
func (r *run) leaseLost() bool {
	return stderrors.Is(context.Cause(r.ctx), errors.ErrRunLeaseLost)
}

// noteLeaseLost stops the run when a fenced write reports the lease is gone.
func (r *run) noteLeaseLost(err error) {
	if stderrors.Is(err, errors.ErrRunLeaseLost) {
		r.cancel(errors.ErrRunLeaseLost)
	}
}

// finish ends the run: it decides the outcome, closes tool parts nobody answered, reports usage, stores the run's end
// with the fenced FinishRun, and returns the terminal event for the handler to write after that.
func (r *run) finish(loopErr error) (model.Event, error) {
	if r.leaseLost() {
		return nil, errors.ErrRunLeaseLost
	}
	status, failure := r.outcome(loopErr)
	r.closeOpenTools(status, failure)
	if status == convmodel.RunStatusCompleted {
		r.emitUsage()
		// Emit may have failed on the usage event itself or tripped a stream cap: re-decide before committing.
		status, failure = r.outcome(nil)
	}
	if err := r.save(status, failure); err != nil {
		return r.saveFailed(err)
	}
	r.svc.deps.Log.InfoContext(r.ctx, logRunFinished, logging.FieldRunID, r.in.Run.ID, fieldStatus, status, fieldSteps, r.currentStep(),
		logging.FieldErrorCode, codeOf(failure), logging.FieldDurationMS, r.svc.deps.Now().Sub(r.started).Milliseconds())
	if status == convmodel.RunStatusFailed {
		return model.ErrorEvent(failure, r.lang), nil
	}
	return model.Finish{}, nil // completed, or cancelled by the user: a Stop is not an error
}

// outcome decides how the run ends. A cap or deadline that cancelled the run context wins, then a failed Emit, so
// `completed` is never committed after either (ruling F2bW2-6). A user Stop only matters when the loop was cut short.
func (r *run) outcome(loopErr error) (convmodel.RunStatus, error) {
	if cause := worker.CauseCode(r.parent); cause != "" {
		return convmodel.RunStatusFailed, errors.New(errors.Code(cause))
	}
	r.mu.Lock()
	emitErr := r.emitErr
	r.mu.Unlock()
	switch {
	case emitErr != nil:
		return convmodel.RunStatusFailed, emitErr
	case loopErr == nil:
		return convmodel.RunStatusCompleted, nil
	case stderrors.Is(loopErr, errors.ErrRunStopped) || stderrors.Is(context.Cause(r.ctx), errors.ErrRunStopped):
		return convmodel.RunStatusCancelled, nil
	case stderrors.Is(loopErr, context.Canceled):
		// The run context ended for no reason the run knows (the worker is going away): the run is interrupted.
		return convmodel.RunStatusFailed, errors.ErrRunInterrupted
	default:
		return convmodel.RunStatusFailed, loopErr
	}
}

// closeOpenTools ends tool parts that never got an output, so a reload never shows a forever-running station.
func (r *run) closeOpenTools(status convmodel.RunStatus, failure error) {
	r.mu.Lock()
	open := make([]string, 0, len(r.open))
	for callID := range r.open {
		open = append(open, callID)
	}
	r.mu.Unlock()
	code := errorCodeCancelled
	if status == convmodel.RunStatusFailed {
		code = codeOf(failure)
	}
	for _, callID := range open {
		r.emit(model.ToolOutputError{CallID: callID, ErrorText: code})
	}
}

func (r *run) emitUsage() {
	r.mu.Lock()
	r.usage.DurationMS = r.svc.deps.Now().Sub(r.started).Milliseconds()
	r.usage.TTFTMS = r.ttft.Milliseconds()
	r.usage.ContextWindow = r.contextWindow()
	usage := r.usage
	r.mu.Unlock()
	r.emit(model.Usage{RunUsage: usage})
}

// save stores the message and ends the run in one fenced FinishRun. A run that streamed nothing and did not complete
// stores no assistant message, only the run row.
func (r *run) save(status convmodel.RunStatus, failure error) error {
	ctx, cancel := r.writeContext()
	defer cancel()
	r.mu.Lock()
	row := r.in.Run
	row.Status = status
	row.ModelRef = r.resolved.ModelRef
	row.InputTokens, row.OutputTokens = r.usage.InputTokens, r.usage.OutputTokens
	var assistant convmodel.Message
	if status == convmodel.RunStatusCompleted || !r.builder.Empty() {
		parts, err := r.builder.Parts()
		if err != nil {
			r.mu.Unlock()
			return err
		}
		assistant = convmodel.Message{ID: row.AssistantMessageID, Role: roleAssistant, Parts: parts, Metadata: r.builder.Metadata()}
	}
	r.mu.Unlock()
	if status == convmodel.RunStatusFailed {
		row.ErrorCode = codeOf(failure)
	}
	return r.svc.deps.Store.FinishRun(ctx, r.in.Lease, row, assistant)
}

// saveFailed handles a FinishRun that did not commit. A lost lease writes nothing more. Anything else gets one bounded
// attempt to close the run row alone as failed, so the conversation is not held by a `running` row. If that fails
// too, the error goes back to the handler, whose safety net ends the run (the row's deadline would free it later).
func (r *run) saveFailed(saveErr error) (model.Event, error) {
	if stderrors.Is(saveErr, errors.ErrRunLeaseLost) {
		return nil, errors.ErrRunLeaseLost
	}
	r.svc.deps.Log.ErrorContext(r.ctx, logRunSaveError, logging.FieldRunID, r.in.Run.ID, logging.FieldError, saveErr.Error())
	if closeErr := r.closeOnly(); closeErr != nil {
		if stderrors.Is(closeErr, errors.ErrRunLeaseLost) {
			return nil, errors.ErrRunLeaseLost
		}
		r.svc.deps.Log.ErrorContext(r.ctx, logRunCloseError, logging.FieldRunID, r.in.Run.ID, logging.FieldError, closeErr.Error())
		return nil, errors.ErrInternalError.WithCause(closeErr)
	}
	// The answer is not stored, so the stream must not claim success: a reload would show nothing.
	return model.ErrorEvent(errors.ErrInternalError.WithCause(saveErr), r.lang), nil
}

// closeOnly closes the run row with no message on its own write budget: the first FinishRun may have spent the whole
// timeout (Codex round 2 #2), and reusing its context would fail at once.
func (r *run) closeOnly() error {
	ctx, cancel := r.writeContext()
	defer cancel()
	row := r.in.Run
	row.Status, row.ErrorCode, row.ModelRef = convmodel.RunStatusFailed, string(errors.CodeInternalError), r.resolved.ModelRef
	return r.svc.deps.Store.FinishRun(ctx, r.in.Lease, row, convmodel.Message{})
}

// writeContext bounds one persistence write by FinishTimeout and survives the run context's cancellation (Stop,
// deadline): the partial answer is still saved.
func (r *run) writeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.parent), r.svc.deps.Config.FinishTimeout)
}

// recordStep stores one run_steps row. A lost lease stops the run; any other storage failure is logged and never
// fails it.
func (r *run) recordStep(kind convmodel.StepKind, name string, outcome convmodel.StepOutcome, err error, usage genkitx.Usage, started time.Time) {
	r.mu.Lock()
	r.step++
	step := convmodel.RunStep{
		RunID: r.in.Run.ID, Step: r.step, Kind: kind, Name: name, Outcome: outcome, ErrorCode: stepErrorCode(err),
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		DurationMS: r.svc.deps.Now().Sub(started).Milliseconds(), StartedAt: started,
	}
	r.mu.Unlock()
	ctx, cancel := r.writeContext()
	defer cancel()
	if storeErr := r.svc.deps.Store.RecordStep(ctx, r.in.Lease, step); storeErr != nil {
		r.noteLeaseLost(storeErr)
		r.svc.deps.Log.WarnContext(ctx, logStepError, logging.FieldRunID, r.in.Run.ID, logging.FieldError, storeErr.Error())
	}
}

// stepErrorCode is the error_code of a run_steps row: the catalog code, or CANCELLED for a cut-short step.
func stepErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case stderrors.Is(err, context.Canceled):
		return errorCodeCancelled
	default:
		return string(errors.CodeOf(err))
	}
}

func (r *run) addUsage(u genkitx.Usage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usage.InputTokens += u.InputTokens
	r.usage.OutputTokens += u.OutputTokens
	if u.CostUSD != nil {
		total := *u.CostUSD
		if r.usage.CostUSD != nil {
			total += *r.usage.CostUSD
		}
		r.usage.CostUSD = &total
	}
}

func (r *run) currentStep() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.step
}

// loop is R4: one model step, then the step's tools in parallel, until the model answers without tools. The stop key
// is checked before each model step and each tool batch (PW-2b4).
func (r *run) loop(messages []*ai.Message) error {
	for step := 0; ; step++ {
		if step >= r.svc.deps.Config.MaxSteps {
			return errors.ErrRunStepLimit
		}
		if r.stopRequested() {
			return errors.ErrRunStopped
		}
		result, err := r.modelStep(messages, r.toolRefs())
		if err != nil {
			return err
		}
		if r.leaseLost() {
			return errors.ErrRunLeaseLost
		}
		messages = append(messages, result.response.Message)
		requests := result.response.ToolRequests()
		if len(requests) == 0 {
			return nil
		}
		if r.stopRequested() {
			return errors.ErrRunStopped
		}
		if !result.reasoned {
			r.status(r.svc.tools.statusFor(requests[0].ToolRequest.Name))
		}
		messages = append(messages, ai.NewMessage(ai.RoleTool, nil, r.runTools(requests)...))
	}
}

func (r *run) toolRefs() []ai.ToolRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.kbDropped || r.agent.KBMissing {
		return nil
	}
	return r.svc.tools.refs
}

func (r *run) dropKB() {
	r.mu.Lock()
	already := r.kbDropped
	r.kbDropped = true
	r.mu.Unlock()
	if !already {
		r.status(model.StatusKBDeleted)
	}
}

// codeOf guards errors.CodeOf, which panics on nil (index amendment 6).
func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return string(errors.CodeOf(err))
}

// contextWindow is R18: the chat model's window, or the configured default when the provider did not report one.
func (r *run) contextWindow() int {
	if r.resolved.ContextWindow != nil && *r.resolved.ContextWindow > 0 {
		return *r.resolved.ContextWindow
	}
	return r.svc.deps.Config.DefaultContextWindow
}
