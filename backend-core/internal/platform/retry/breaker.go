package retry

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/sony/gobreaker/v2"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const breakerErrorFormat = "circuit breaker execute: %w"

type Breaker struct {
	breaker  *gobreaker.CircuitBreaker[struct{}]
	settings config.BreakerConfig
}

func NewBreaker(name string, settings config.BreakerConfig) *Breaker {
	return &Breaker{settings: settings, breaker: gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        name,
		MaxRequests: settings.HalfOpenMaxCalls,
		Timeout:     settings.OpenTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= settings.FailureThreshold },
		IsSuccessful: func(err error) bool {
			return err == nil || !IsRetryable(err)
		},
	})}
}

func (b *Breaker) ForProvider(name string) *Breaker { return NewBreaker(name, b.settings) }

func (b *Breaker) CanDiscard() bool {
	return b.breaker.State() == gobreaker.StateClosed && b.breaker.Counts().ConsecutiveFailures == 0
}

func (b *Breaker) Execute(ctx context.Context, op func(context.Context) error) error {
	_, err := b.breaker.Execute(func() (struct{}, error) { return struct{}{}, op(ctx) })
	if stderrors.Is(err, gobreaker.ErrOpenState) || stderrors.Is(err, gobreaker.ErrTooManyRequests) {
		return errors.ErrProviderUnavailable.WithCause(err)
	}
	if err != nil {
		return fmt.Errorf(breakerErrorFormat, err)
	}
	return nil
}
