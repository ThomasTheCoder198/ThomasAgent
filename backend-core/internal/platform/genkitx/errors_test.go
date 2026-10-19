package genkitx_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/firebase/genkit/go/core/status"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
)

func TestToAppError_MapsGenkitStatusesToCatalogCodes(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{status.Errorf(status.ErrUnauthenticated, "bad key"), errors.ErrLLMProviderRejected},
		{status.Errorf(status.ErrPermissionDenied, "denied"), errors.ErrLLMProviderRejected},
		{status.Errorf(status.ErrNotFound, "no such model"), errors.ErrLLMProviderRejected},
		{status.Errorf(status.ErrInvalidArgument, "bad request"), errors.ErrLLMProviderRejected},
		{status.Errorf(status.ErrResourceExhausted, "429"), errors.ErrProviderUnavailable},
		{status.Errorf(status.ErrUnavailable, "503"), errors.ErrProviderUnavailable},
		{status.Errorf(status.ErrDeadlineExceeded, "slow"), errors.ErrDependencyTimeout},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), errors.ErrDependencyTimeout},
		{fmt.Errorf("plain"), errors.ErrProviderUnavailable},
	}
	for _, c := range cases {
		require.ErrorIs(t, genkitx.ToAppError(c.in), c.want, c.in.Error())
	}
	require.ErrorIs(t, genkitx.ToAppError(fmt.Errorf("x: %w", context.Canceled)), context.Canceled)
	require.NoError(t, genkitx.ToAppError(nil))
	app := errors.ErrKbNotFound.WithMessage("kept")
	require.Same(t, app, genkitx.ToAppError(app))
}
