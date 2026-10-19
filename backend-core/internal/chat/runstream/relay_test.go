package runstream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
)

func TestRelayEnd(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	finish := 10 * time.Second
	row := func(status convmodel.RunStatus, code string, deadline time.Time) convmodel.Run {
		return convmodel.Run{Status: status, ErrorCode: code, DeadlineAt: deadline}
	}
	cases := []struct {
		name    string
		outcome Outcome
		row     convmodel.Run
		want    model.Event
	}{
		{"the worker's own terminal event ends with finish", TerminalRelayed, row(convmodel.RunStatusCompleted, "", now.Add(time.Minute)), model.Finish{}},
		{"completed row without a relayed terminal", NoTerminal, row(convmodel.RunStatusCompleted, "", now.Add(time.Minute)), model.Error{Code: "RUN_STREAM_INCOMPLETE"}},
		{"cancelled row without a relayed terminal", RowEnded, row(convmodel.RunStatusCancelled, "", now.Add(time.Minute)), model.Error{Code: "RUN_STREAM_INCOMPLETE"}},
		{"failed row reports its own code", RowEnded, row(convmodel.RunStatusFailed, "RUN_OUTPUT_TOO_LARGE", now.Add(time.Minute)), model.Error{Code: "RUN_OUTPUT_TOO_LARGE"}},
		{"failed row with no code is masked", RowEnded, row(convmodel.RunStatusFailed, "", now.Add(time.Minute)), model.Error{Code: "INTERNAL_ERROR"}},
		{"running past deadline plus finish is interrupted", NoTerminal, row(convmodel.RunStatusRunning, "", now.Add(-finish-time.Second)), model.Error{Code: "RUN_INTERRUPTED"}},
		{"running inside the window cannot be proven delivered", NoTerminal, row(convmodel.RunStatusRunning, "", now.Add(-finish/2)), model.Error{Code: "RUN_STREAM_INCOMPLETE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RelayEnd(tc.outcome, tc.row, now, finish)
			if want, ok := tc.want.(model.Error); ok {
				gotErr, isErr := got.(model.Error)
				require.True(t, isErr, "want an error part, got %T", got)
				require.Equal(t, want.Code, gotErr.Code)
				require.NotEmpty(t, gotErr.Message, "the part carries the catalog message")
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
}
