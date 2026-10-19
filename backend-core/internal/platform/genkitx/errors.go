package genkitx

import (
	"context"
	stderrors "errors"

	"github.com/firebase/genkit/go/core/status"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// ToAppError maps a Genkit or provider failure to the catalog. Cancellation passes through unchanged, because the
// caller decides whether it was Stop or a deadline. Catalog errors pass through as they are.
func ToAppError(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, context.Canceled) {
		return err
	}
	var app *errors.AppError
	if stderrors.As(err, &app) {
		return app
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return errors.ErrDependencyTimeout.WithCause(err)
	}
	name, _ := status.Classified(err)
	switch name {
	case status.Unauthenticated, status.PermissionDenied, status.NotFound, status.InvalidArgument,
		status.FailedPrecondition, status.OutOfRange:
		return errors.ErrLLMProviderRejected.WithCause(err)
	case status.DeadlineExceeded:
		return errors.ErrDependencyTimeout.WithCause(err)
	default:
		return errors.ErrProviderUnavailable.WithCause(err)
	}
}
