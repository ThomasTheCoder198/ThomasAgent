package retry

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type attemptTimeout struct{}

func (attemptTimeout) Error() string   { return "attempt timed out" }
func (attemptTimeout) Timeout() bool   { return true }
func (attemptTimeout) Temporary() bool { return true }

func TestDo_RetriesPerAttemptTimeoutWhileParentContextAlive(t *testing.T) {
	for _, failure := range []error{fmt.Errorf("http client: %w", context.DeadlineExceeded), attemptTimeout{}} {
		t.Run(failure.Error(), func(t *testing.T) {
			sleeps := &recordedSleeps{}
			policy := testPolicy(sleeps)
			calls := 0
			err := Do(t.Context(), policy, func(ctx context.Context) error {
				require.NoError(t, ctx.Err())
				calls++
				if calls < policy.MaxAttempts {
					return failure
				}
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, policy.MaxAttempts, calls)
			require.Len(t, sleeps.got, policy.MaxAttempts-1)
		})
	}
}

func TestDo_StopsWhenParentContextCancelled(t *testing.T) {
	for _, operationError := range []error{nil, errors.ErrRateLimited, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(operationError), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			sleeps := &recordedSleeps{}
			cause := fmt.Errorf("caller stopped")
			calls := 0
			err := Do(ctx, testPolicy(sleeps), func(context.Context) error { calls++; cancel(cause); return operationError })
			if operationError == nil {
				require.NoError(t, err)
			} else {
				var appErr *errors.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, errors.CodeProviderUnavailable, appErr.Code)
				require.ErrorIs(t, err, cause)
				require.ErrorIs(t, err, operationError)
			}
			require.Equal(t, 1, calls)
			require.Empty(t, sleeps.got)
		})
	}
}
