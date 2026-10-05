package retry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	b := NewBreaker("test", config.BreakerConfig{FailureThreshold: 2, OpenTimeout: time.Minute, HalfOpenMaxCalls: 1})
	fail := func(context.Context) error { return errors.New(errors.CodeProviderUnavailable) }
	_ = b.Execute(context.Background(), fail)
	_ = b.Execute(context.Background(), fail)
	err := b.Execute(context.Background(), func(context.Context) error { return nil })
	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, errors.CodeProviderUnavailable, appErr.Code)
}

func TestDoStopsForCanceledContextAndInvalidAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := Do(ctx, Policy{RetryConfig: config.RetryConfig{MaxAttempts: 3}, Rand: func() float64 { return 0 }, Sleep: func(context.Context, time.Duration) error { return nil }}, func(context.Context) error {
		calls++
		return errors.ErrProviderUnavailable
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
	err = Do(context.Background(), Policy{RetryConfig: config.RetryConfig{MaxAttempts: 0}}, func(context.Context) error {
		t.Fatal("operation called with no attempts")
		return nil
	})
	require.Error(t, err)
}

func TestDoRejectsNonpositiveAttempts(t *testing.T) {
	for _, attempts := range []int{0, -1} {
		err := Do(t.Context(), Policy{RetryConfig: config.RetryConfig{MaxAttempts: attempts}}, func(context.Context) error { t.Fatal("invalid policy ran the operation"); return nil })
		require.Error(t, err)
	}
}

func TestBreaker_NonRetryableErrorsLeaveClosed(t *testing.T) {
	for _, code := range []errors.Code{errors.CodeValidationFailed, errors.CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			b := NewBreaker("client-errors", config.BreakerConfig{FailureThreshold: 5, OpenTimeout: time.Minute, HalfOpenMaxCalls: 1})
			failure := errors.New(code)
			for range 6 {
				require.ErrorIs(t, b.Execute(context.Background(), func(context.Context) error { return failure }), failure)
			}
			require.NoError(t, b.Execute(context.Background(), func(context.Context) error { return nil }))
		})
	}
}

func TestNewBreaker_UsesConfig(t *testing.T) {
	settings := config.BreakerConfig{FailureThreshold: 7, OpenTimeout: 45 * time.Second, HalfOpenMaxCalls: 3}
	breaker := NewBreaker("provider", settings)
	require.Equal(t, "provider", breaker.breaker.Name())
	failure := func(context.Context) error { return errors.ErrProviderUnavailable }
	for range settings.FailureThreshold {
		require.ErrorIs(t, breaker.Execute(context.Background(), failure), errors.ErrProviderUnavailable)
	}
	require.ErrorIs(t, breaker.Execute(context.Background(), func(context.Context) error { return nil }), errors.ErrProviderUnavailable)
}
