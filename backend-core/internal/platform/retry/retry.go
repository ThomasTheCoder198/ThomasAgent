package retry

import (
	"context"
	stderrors "errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const (
	backoffBase      = 2
	sleepErrorFormat = "retry wait: %w"
	retryTracer      = "core.retry"
	retryOperation   = "core.retry.execute"
)

type Policy struct {
	config.RetryConfig
	Sleep func(context.Context, time.Duration) error
	Rand  func() float64
}

type RetryAfterProvider interface {
	RetryAfter() time.Duration
}

func NewPolicy(c config.RetryConfig) Policy {
	return Policy{RetryConfig: c, Sleep: sleepWithContext, Rand: rand.Float64}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf(sleepErrorFormat, ctx.Err())
	case <-t.C:
		return nil
	}
}

func IsRetryable(err error) bool {
	if stderrors.Is(err, context.Canceled) {
		return false
	}
	var appErr *errors.AppError
	if stderrors.As(err, &appErr) {
		return appErr.Retryable()
	}
	var ra RetryAfterProvider
	if stderrors.As(err, &ra) {
		return true
	}
	var netErr net.Error
	if stderrors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	return stderrors.As(err, &opErr)
}

func Backoff(p Policy, attempt int) time.Duration {
	ceiling := float64(p.BaseDelay) * math.Pow(backoffBase, float64(attempt))
	if ceiling > float64(p.MaxDelay) {
		ceiling = float64(p.MaxDelay)
	}
	return time.Duration(p.Rand() * ceiling)
}

func retryDelay(p Policy, attempt int, err error) time.Duration {
	delay := Backoff(p, attempt)
	var provider RetryAfterProvider
	if stderrors.As(err, &provider) && provider.RetryAfter() > 0 {
		// The local backoff cap cannot shorten a provider's requested minimum wait.
		delay = max(delay, provider.RetryAfter())
	}
	return delay
}

func Do(ctx context.Context, p Policy, op func(context.Context) error) error {
	ctx, span := otel.Tracer(retryTracer).Start(ctx, retryOperation)
	defer span.End()
	if p.MaxAttempts <= 0 {
		return errors.ErrInternalError.WithCause(fmt.Errorf("retry max attempts must be positive"))
	}
	var err error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.ErrProviderUnavailable.WithCause(context.Cause(ctx))
		}
		err = op(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errors.ErrProviderUnavailable.WithCause(stderrors.Join(err, context.Cause(ctx)))
		}
		if !IsRetryable(err) {
			return terminalError(err)
		}
		if attempt == p.MaxAttempts-1 {
			break
		}
		delay := retryDelay(p, attempt, err)
		if sleepErr := p.Sleep(ctx, delay); sleepErr != nil {
			return errors.ErrProviderUnavailable.WithCause(stderrors.Join(err, sleepErr, context.Cause(ctx)))
		}
	}
	return terminalError(err)
}

func terminalError(err error) error {
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return errors.ErrProviderUnavailable.WithCause(err)
	}
	return err
}
