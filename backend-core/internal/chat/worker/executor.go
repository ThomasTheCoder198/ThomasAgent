package worker

import (
	"context"
	stderrors "errors"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/control"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// Executor runs a claimed chat run (2b-4 implements it). It owns the run's database writes: it records steps with
// RecordStep(lease, …) and ends the run with the fenced FinishRun(lease, …). It returns the terminal event for the
// handler to write once FinishRun committed: Finish, or an Error naming the failure. A lost lease is reported as
// errors.ErrRunLeaseLost (the handler then writes nothing; whoever ended the run writes its terminal event).
//
// The ctx ends when the run deadline passes, the heartbeat fails, or the stream cap is exceeded; context.Cause tells
// which, and CauseCode gives the catalog code to finish a failed run with. A user Stop does not cancel the ctx: it
// arrives through ExecuteInput.Control, to be checked between steps and before each tool batch.
type Executor interface {
	Execute(ctx context.Context, in ExecuteInput) (model.Event, error)
}

// ExecuteInput is everything an executor gets for one claimed run.
type ExecuteInput struct {
	Run   convmodel.Run
	Lease convmodel.Lease
	// Emit writes one non-terminal event to the run's stream, enforcing the event and byte caps. It is safe to call
	// from several goroutines and keeps working after ctx is cancelled, so a partial answer can still be closed.
	Emit    func(model.Event) error
	Control control.Subscription
}

// CauseCode is the catalog code a cancelled run ctx carries, for a failed run's error_code: RUN_OUTPUT_TOO_LARGE or
// RUN_DEADLINE_EXCEEDED. It is "" for a ctx that is not cancelled and for a lost lease (nothing may be written then).
func CauseCode(ctx context.Context) string {
	cause := context.Cause(ctx)
	switch {
	case stderrors.Is(cause, errors.ErrRunOutputTooLarge):
		return string(errors.CodeRunOutputTooLarge)
	case stderrors.Is(cause, errors.ErrRunDeadlineExceeded):
		return string(errors.CodeRunDeadlineExceeded)
	default:
		return ""
	}
}
