package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
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

func NewBreaker(s BreakerSettings) *Breaker {
	return &Breaker{cb: gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        s.Name,
		MaxRequests: s.HalfOpenMaxCalls,
		Timeout:     s.OpenTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= s.FailureThreshold },
	})}
}

func (b *Breaker) Execute(ctx context.Context, op func(context.Context) error) error {
	_, err := b.cb.Execute(func() (struct{}, error) { return struct{}{}, op(ctx) })
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return apperr.New(apperr.CodeProviderUnavailable, apperr.WithCause(err))
	}
	if err != nil {
		return fmt.Errorf(breakerErrorFormat, err)
	}
	return nil
}
