package tenant

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const PlatformID = "default"

type contextKey struct{}

var ErrMissingTenant = errors.ErrInternalError

// WithID is for trusted server code; tenant IDs never come from request JSON or model output.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// ID fails closed so omitted boundary setup cannot select platform data.
func ID(ctx context.Context) (string, error) {
	if id, ok := ctx.Value(contextKey{}).(string); ok && id != "" {
		return id, nil
	}
	return "", ErrMissingTenant
}
