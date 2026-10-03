package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"

	catalogerrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const breakerErrorFormat = "circuit breaker execute: %w"

type BreakerSettings struct {
	Name             string
	FailureThreshold uint32
	OpenTimeout      time.Duration
	HalfOpenMaxCalls uint32
}

type Breaker struct {
	cb *gobreaker.CircuitBreaker[struct{}]
}

func BreakerFromConfig(name string, c config.BreakerConfig) BreakerSettings {
	return BreakerSettings{Name: name, FailureThreshold: c.FailureThreshold, OpenTimeout: c.OpenTimeout, HalfOpenMaxCalls: c.HalfOpenMaxCalls}
}

func NewBreaker(s BreakerSettings) *Breaker {
	return &Breaker{cb: gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        s.Name,
		MaxRequests: s.HalfOpenMaxCalls,
		Timeout:     s.OpenTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= s.FailureThreshold },
		IsSuccessful: func(err error) bool {
			return err == nil || !IsRetryable(err)
		},
	})}
}

func (b *Breaker) Execute(ctx context.Context, op func(context.Context) error) error {
	_, err := b.cb.Execute(func() (struct{}, error) { return struct{}{}, op(ctx) })
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return catalogerrors.ErrProviderUnavailable.WithCause(err)
	}
	if err != nil {
		return fmt.Errorf(breakerErrorFormat, err)
	}
	return nil
}
