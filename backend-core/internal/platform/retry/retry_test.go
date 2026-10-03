package retry

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	catalogerrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

type recordedSleeps struct{ got []time.Duration }

func (r *recordedSleeps) sleep(_ context.Context, d time.Duration) error {
	r.got = append(r.got, d)
	return nil
}

func testPolicy(s *recordedSleeps) Policy {
	return Policy{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second, Sleep: s.sleep, Rand: func() float64 { return 1 }}
}

type retryAfterErr struct{ d time.Duration }

func (e retryAfterErr) Error() string             { return "slow down" }
func (e retryAfterErr) RetryAfter() time.Duration { return e.d }

type retryAfterWrapper struct{ error }

func (e retryAfterWrapper) Unwrap() error             { return e.error }
func (e retryAfterWrapper) RetryAfter() time.Duration { return time.Second }

func TestNamedErrorRetryability(t *testing.T) {
	require.True(t, IsRetryable(catalogerrors.ErrProviderUnavailable))
	require.True(t, IsRetryable(retryAfterWrapper{error: catalogerrors.ErrRateLimited}))
	require.False(t, IsRetryable(catalogerrors.ErrValidationFailed))
	require.False(t, IsRetryable(retryAfterWrapper{error: catalogerrors.ErrNotFound}))
}

func TestRetryAfterDoesNotOverrideCatalogRetryability(t *testing.T) {
	for _, code := range []apperr.Code{apperr.CodeValidationFailed, apperr.CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			wrapped := retryAfterWrapper{error: apperr.New(code)}
			require.False(t, IsRetryable(wrapped))
			s := &recordedSleeps{}
			calls := 0
			err := Do(context.Background(), testPolicy(s), func(context.Context) error {
				calls++
				return wrapped
			})
			require.ErrorIs(t, err, wrapped.error)
			require.Equal(t, 1, calls)
			require.Empty(t, s.got)
		})
	}
}

func TestDoRetriesRetryableUntilSuccess(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		if calls < 3 {
			return apperr.New(apperr.CodeProviderUnavailable)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, s.got)
}

func TestDoStopsOnNonRetryable(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		return apperr.New(apperr.CodeValidationFailed)
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestDoGivesUpAfterMaxAttempts(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		return apperr.New(apperr.CodeRateLimited)
	})
	require.Error(t, err)
	require.Equal(t, 4, calls)
}

func TestDoHonorsRetryAfter(t *testing.T) {
	s := &recordedSleeps{}
	p := testPolicy(s)
	p.MaxDelay = 5 * time.Second
	calls := 0
	_ = Do(context.Background(), p, func(context.Context) error {
		calls++
		if calls == 1 {
			return retryAfterErr{d: 3 * time.Second}
		}
		return nil
	})
	require.Equal(t, []time.Duration{3 * time.Second}, s.got)
}

func TestDoCapsRetryAfterAtMaxDelay(t *testing.T) {
	s := &recordedSleeps{}
	p := testPolicy(s)
	calls := 0
	err := Do(context.Background(), p, func(context.Context) error {
		calls++
		if calls == 1 {
			return retryAfterErr{d: time.Hour}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []time.Duration{p.MaxDelay}, s.got)
}

func TestBackoffIsCappedAndJittered(t *testing.T) {
	p := Policy{BaseDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond, Rand: func() float64 { return 0.5 }}
	require.Equal(t, 50*time.Millisecond, Backoff(p, 0))
	require.Equal(t, 150*time.Millisecond, Backoff(p, 5))
}

func TestIsRetryable(t *testing.T) {
	require.True(t, IsRetryable(apperr.New(apperr.CodeRateLimited)))
	require.False(t, IsRetryable(apperr.New(apperr.CodeNotFound)))
	require.True(t, IsRetryable(&net.OpError{Op: "dial", Err: errors.New("refused")}))
	require.False(t, IsRetryable(context.Canceled))
}

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := NewBreaker(BreakerSettings{Name: "test", FailureThreshold: 2, OpenTimeout: time.Minute, HalfOpenMaxCalls: 1})
	fail := func(context.Context) error { return apperr.New(apperr.CodeProviderUnavailable) }
	_ = b.Execute(context.Background(), fail)
	_ = b.Execute(context.Background(), fail)
	err := b.Execute(context.Background(), func(context.Context) error { return nil })
	var appErr *apperr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperr.CodeProviderUnavailable, appErr.Code)
}

func TestBreakerNonRetryableErrorsLeaveClosed(t *testing.T) {
	for _, code := range []apperr.Code{apperr.CodeValidationFailed, apperr.CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			b := NewBreaker(BreakerSettings{Name: "client-errors", FailureThreshold: 5, OpenTimeout: time.Minute, HalfOpenMaxCalls: 1})
			failure := apperr.New(code)
			for range 6 {
				require.ErrorIs(t, b.Execute(context.Background(), func(context.Context) error { return failure }), failure)
			}
			require.NoError(t, b.Execute(context.Background(), func(context.Context) error { return nil }))
		})
	}
}

func TestBreakerFromConfig(t *testing.T) {
	c := config.BreakerConfig{FailureThreshold: 7, OpenTimeout: 45 * time.Second, HalfOpenMaxCalls: 3}
	require.Equal(t, BreakerSettings{Name: "provider", FailureThreshold: 7, OpenTimeout: 45 * time.Second, HalfOpenMaxCalls: 3}, BreakerFromConfig("provider", c))
}
