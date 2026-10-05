package retry

import (
	"context"
	stderrors "errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

type recordedSleeps struct{ got []time.Duration }

func (r *recordedSleeps) sleep(_ context.Context, d time.Duration) error {
	r.got = append(r.got, d)
	return nil
}

func testPolicy(s *recordedSleeps) Policy {
	return Policy{RetryConfig: config.RetryConfig{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}, Sleep: s.sleep, Rand: func() float64 { return 1 }}
}

type retryAfterErr struct{ d time.Duration }

func (e retryAfterErr) Error() string             { return "slow down" }
func (e retryAfterErr) RetryAfter() time.Duration { return e.d }

type retryAfterWrapper struct{ error }

func (e retryAfterWrapper) Unwrap() error             { return e.error }
func (e retryAfterWrapper) RetryAfter() time.Duration { return time.Second }

func TestIsRetryable_NamedErrors(t *testing.T) {
	require.True(t, IsRetryable(errors.ErrProviderUnavailable))
	require.True(t, IsRetryable(retryAfterWrapper{error: errors.ErrRateLimited}))
	require.False(t, IsRetryable(errors.ErrValidationFailed))
	require.False(t, IsRetryable(retryAfterWrapper{error: errors.ErrNotFound}))
}

func TestDo_RetryAfterDoesNotOverrideCatalogRetryability(t *testing.T) {
	for _, code := range []errors.Code{errors.CodeValidationFailed, errors.CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			wrapped := retryAfterWrapper{error: errors.New(code)}
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

func TestDo_RetriesRetryableUntilSuccess(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New(errors.CodeProviderUnavailable)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, s.got)
}

func TestDo_StopsOnNonRetryable(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		return errors.New(errors.CodeValidationFailed)
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestDo_GivesUpAfterMaxAttempts(t *testing.T) {
	s := &recordedSleeps{}
	calls := 0
	err := Do(context.Background(), testPolicy(s), func(context.Context) error {
		calls++
		return errors.New(errors.CodeRateLimited)
	})
	require.Error(t, err)
	require.Equal(t, 4, calls)
}

func TestDo_HonorsRetryAfter(t *testing.T) {
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

func TestDo_ProviderDelayRemainsContextCancellable(t *testing.T) {
	s := &recordedSleeps{}
	p := testPolicy(s)
	p.Sleep = func(_ context.Context, delay time.Duration) error {
		s.got = append(s.got, delay)
		return context.Canceled
	}
	calls := 0
	err := Do(context.Background(), p, func(context.Context) error {
		calls++
		if calls == 1 {
			return retryAfterErr{d: time.Hour}
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	require.Equal(t, []time.Duration{time.Hour}, s.got)
}

func TestDo_HonorsRetryAfterBeyondBackoffCap(t *testing.T) {
	sleeps := &recordedSleeps{}
	policy := testPolicy(sleeps)
	calls := 0
	err := Do(t.Context(), policy, func(context.Context) error {
		calls++
		if calls == 1 {
			return retryAfterErr{d: time.Minute}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, []time.Duration{time.Minute}, sleeps.got)
}

func TestBackoff_IsCappedAndJittered(t *testing.T) {
	p := Policy{RetryConfig: config.RetryConfig{BaseDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond}, Rand: func() float64 { return 0.5 }}
	require.Equal(t, 50*time.Millisecond, Backoff(p, 0))
	require.Equal(t, 150*time.Millisecond, Backoff(p, 5))
}

func TestIsRetryable_ClassifiesFailures(t *testing.T) {
	require.True(t, IsRetryable(errors.New(errors.CodeRateLimited)))
	require.False(t, IsRetryable(errors.New(errors.CodeNotFound)))
	require.True(t, IsRetryable(&net.OpError{Op: "dial", Err: stderrors.New("refused")}))
	require.False(t, IsRetryable(context.Canceled))
}

func TestDo_ContextFailuresUseCatalogErrorsAndPreserveCause(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(cause)
			err := Do(ctx, testPolicy(&recordedSleeps{}), func(context.Context) error {
				t.Fatal("canceled context executed operation")
				return nil
			})
			var appErr *errors.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, errors.CodeProviderUnavailable, appErr.Code)
			require.ErrorIs(t, err, cause)
		})
	}
}

func TestDo_WaitFailureUsesCatalogErrorAndRetainsOperation(t *testing.T) {
	policy := testPolicy(&recordedSleeps{})
	policy.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
	err := Do(t.Context(), policy, func(context.Context) error { return errors.ErrRateLimited })
	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, errors.CodeProviderUnavailable, appErr.Code)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errors.ErrRateLimited)
}

func TestDo_OperationCancellationUsesCatalogError(t *testing.T) {
	err := Do(t.Context(), testPolicy(&recordedSleeps{}), func(context.Context) error { return context.Canceled })
	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, errors.CodeProviderUnavailable, appErr.Code)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDo_EmitsRetrySpanWithParentContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	ctx, parent := provider.Tracer("test").Start(t.Context(), "parent")
	err := Do(ctx, testPolicy(&recordedSleeps{}), func(context.Context) error { return nil })
	parent.End()
	require.NoError(t, err)
	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.Equal(t, "core.retry.execute", spans[0].Name())
	require.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
}
