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
