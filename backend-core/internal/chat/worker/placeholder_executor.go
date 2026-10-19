package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

// FinishStore is the fenced end of a run, which the executor owns.
type FinishStore interface {
	FinishRun(ctx context.Context, lease convmodel.Lease, run convmodel.Run, assistant convmodel.Message) error
}

// PlaceholderExecutor is TEMPORARY: replaced by 2b-4, which supplies the run loop: it ends every run it is given as failed with
// RUN_INTERRUPTED and says so in the stream. The chat endpoint (2b-5) does not exist yet, so no run reaches it in
// production; it lets the worker path run end to end on a real stack today.
type PlaceholderExecutor struct {
	Store         FinishStore
	FinishTimeout time.Duration
	Log           *slog.Logger
}

func (e PlaceholderExecutor) Execute(ctx context.Context, in ExecuteInput) (model.Event, error) {
	// One warning per run, so nobody mistakes this for the real run loop.
	e.Log.WarnContext(ctx, "chat run ended by the placeholder executor; the real run loop arrives in 2b-4", logging.FieldRunID, in.Run.ID)
	if err := in.Emit(model.Start{MessageID: in.Run.AssistantMessageID, RunID: in.Run.ID}); err != nil {
		return nil, err
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.FinishTimeout)
	defer cancel()
	run := in.Run
	run.Status, run.ErrorCode = convmodel.RunStatusFailed, string(errors.CodeRunInterrupted)
	if err := e.Store.FinishRun(finishCtx, in.Lease, run, convmodel.Message{}); err != nil {
		return nil, err
	}
	return model.ErrorEvent(errors.ErrRunInterrupted, terminalLang), nil
}
