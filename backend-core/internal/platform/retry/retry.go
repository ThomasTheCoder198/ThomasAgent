package retry

import (
	"context"
	stderrors "errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const (
	backoffBase      = 2
	sleepErrorFormat = "retry wait: %w"
)

type Policy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Sleep       func(context.Context, time.Duration) error
	Rand        func() float64
}

type RetryAfterer interface {
	RetryAfter() time.Duration
}

func FromConfig(c config.RetryConfig) Policy {
	return Policy{MaxAttempts: c.MaxAttempts, BaseDelay: c.BaseDelay, MaxDelay: c.MaxDelay, Sleep: sleepCtx, Rand: rand.Float64}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
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
	var ra RetryAfterer
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

func Do(ctx context.Context, p Policy, op func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if err = op(ctx); err == nil || !IsRetryable(err) {
			return err
		}
		if attempt == p.MaxAttempts-1 {
			break
		}
		delay := Backoff(p, attempt)
		var ra RetryAfterer
		if stderrors.As(err, &ra) && ra.RetryAfter() > 0 {
			delay = min(ra.RetryAfter(), p.MaxDelay)
		}
		if sleepErr := p.Sleep(ctx, delay); sleepErr != nil {
			return stderrors.Join(err, sleepErr)
		}
	}
	return err
}
