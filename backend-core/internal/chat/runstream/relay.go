package runstream

import (
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// Outcome says how Follow ended.
type Outcome string

const (
	// TerminalRelayed: the worker's own terminal event reached onEvent, so the client has every delta.
	TerminalRelayed Outcome = "terminal_relayed"
	// NoTerminal: the window is over, or Redis failed; delivery of the deltas cannot be proven.
	NoTerminal Outcome = "no_terminal"
	// RowEnded: the run row is no longer running and no terminal event came.
	RowEnded Outcome = "row_ended"
)

// terminalLang words the message of a closing part; the UI stream carries only the code, which the web localizes.
const terminalLang = errors.LangVI

// RelayEnd is the relay policy of amendment W4, a pure function so the DB fallback is testable without a server.
// finish only for TerminalRelayed; without a relayed terminal the relay cannot prove the client got every delta, so
// it ends with an error part chosen from the run row:
//   - completed or cancelled: RUN_STREAM_INCOMPLETE; the client refetches the stored answer, the authoritative copy;
//   - failed: the row's own code;
//   - still running past deadline + finish timeout: RUN_INTERRUPTED; still running inside it: RUN_STREAM_INCOMPLETE.
func RelayEnd(outcome Outcome, row convmodel.Run, now time.Time, finishTimeout time.Duration) model.Event {
	if outcome == TerminalRelayed {
		return model.Finish{}
	}
	switch row.Status {
	case convmodel.RunStatusFailed:
		return model.ErrorEvent(errors.New(errors.Code(row.ErrorCode)), terminalLang)
	case convmodel.RunStatusRunning:
		if now.After(row.DeadlineAt.Add(finishTimeout)) {
			return model.ErrorEvent(errors.ErrRunInterrupted, terminalLang)
		}
	}
	return model.ErrorEvent(errors.ErrRunStreamIncomplete, terminalLang)
}
