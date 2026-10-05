package retry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDo_ReturnsSuccessWhenOperationCancelsParent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sleeps := &recordedSleeps{}
	calls := 0
	err := Do(ctx, testPolicy(sleeps), func(context.Context) error {
		calls++
		cancel()
		return nil
	})
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.NoError(t, err, "caller cancellation must not discard an operation's completed success")
	require.Equal(t, 1, calls)
	require.Empty(t, sleeps.got)
}
