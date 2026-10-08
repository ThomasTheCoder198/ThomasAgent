package tenant

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const PlatformID = "default"

type contextKey struct{}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

func ID(ctx context.Context) (string, error) {
	if id, ok := ctx.Value(contextKey{}).(string); ok && id != "" {
		return id, nil
	}
	return "", errors.ErrMissingTenant
}
