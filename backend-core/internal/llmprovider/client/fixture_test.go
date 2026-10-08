package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const secretKey = "sk-or-v1-supersecret"

func platformContext(ctx context.Context) context.Context {
	return tenant.WithID(ctx, tenant.PlatformID)
}

func code(t *testing.T, err error) errors.Code {
	t.Helper()
	require.Error(t, err)
	return errors.ToAppError(err).Code
}
