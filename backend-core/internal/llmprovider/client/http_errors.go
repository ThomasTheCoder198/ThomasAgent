package client

import (
	"net/http"
	"strconv"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
)

const (
	decimalBase = 10
	intBits     = 64
	// maxRetryAfter caps a provider's Retry-After so a huge value cannot overflow time.Duration.
	maxRetryAfter = time.Duration(1<<63 - 1)
)

// statusError keeps the provider's 5xx status as the cause without exposing its body.
type statusError struct{ status int }

func (e statusError) Error() string { return "provider returned status " + strconv.Itoa(e.status) }

type retryAfterError struct {
	error
	delay time.Duration
}

func (e retryAfterError) RetryAfter() time.Duration { return e.delay }

func (e retryAfterError) Unwrap() error { return e.error }

// withRetryAfter attaches the provider's Retry-After (seconds or HTTP date) to retryable errors.
func withRetryAfter(err error, value string) error {
	if !retry.IsRetryable(err) {
		return err
	}
	if seconds, parseErr := strconv.ParseInt(value, decimalBase, intBits); parseErr == nil && seconds > 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return retryAfterError{err, maxRetryAfter}
		}
		return retryAfterError{err, time.Duration(seconds) * time.Second}
	}
	if deadline, parseErr := http.ParseTime(value); parseErr == nil {
		if delay := time.Until(deadline); delay > 0 {
			return retryAfterError{err, min(delay, maxRetryAfter)}
		}
	}
	return err
}

// errorForStatus: 429 is rate limiting, 5xx is a retryable outage, any other 4xx means the key
// or base URL is wrong.
func errorForStatus(status int) error {
	switch {
	case status == http.StatusTooManyRequests:
		return errors.ErrRateLimited
	case status >= http.StatusInternalServerError:
		return errors.ErrProviderUnavailable.WithCause(statusError{status})
	case status >= http.StatusBadRequest:
		return errors.ErrLLMProviderRejected
	}
	return nil
}
