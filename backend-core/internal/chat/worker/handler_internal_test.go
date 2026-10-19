package worker

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestDecodeJob_NeedsARunAndAUser(t *testing.T) {
	good := `{"runId":"` + uuid.NewString() + `","conversationId":"` + uuid.NewString() + `","userId":"` + uuid.NewString() + `"}`
	job, err := decodeJob([]byte(good))
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, job.RunID)
	for _, payload := range []string{``, `not json`, `{}`, `{"runId":"` + uuid.NewString() + `"}`, `{"userId":"` + uuid.NewString() + `"}`} {
		_, err := decodeJob([]byte(payload))
		require.ErrorIs(t, err, errors.ErrChatRunJobInvalid, payload)
	}
}

func TestFailureCode_PrefersTheCauseThatCancelledTheRun(t *testing.T) {
	plain := stderrors.New("boom")
	require.Equal(t, errors.CodeRunOutputTooLarge, failureCode(plain, errors.ErrRunOutputTooLarge))
	require.Equal(t, errors.CodeRunDeadlineExceeded, failureCode(plain, errors.ErrRunDeadlineExceeded))
	require.Equal(t, errors.CodeRagxUnavailable, failureCode(errors.ErrRagxUnavailable, nil))
	require.Equal(t, errors.CodeInternalError, failureCode(plain, nil), "an unknown error is masked by its code")
}

func TestCauseCode_NamesOnlyCapAndDeadline(t *testing.T) {
	cancelled := func(cause error) context.Context {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		return ctx
	}
	require.Empty(t, CauseCode(context.Background()))
	require.Equal(t, "RUN_OUTPUT_TOO_LARGE", CauseCode(cancelled(errors.ErrRunOutputTooLarge)))
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", CauseCode(cancelled(errors.ErrRunDeadlineExceeded)))
	require.Empty(t, CauseCode(cancelled(errors.ErrRunLeaseLost)), "a lost lease writes nothing")
}
