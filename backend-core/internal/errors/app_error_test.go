package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamedErrorUsesDefinition(t *testing.T) {
	var err error = ErrNotFound
	appErr := From(err)
	require.Equal(t, CodeNotFound, appErr.Code)
	require.Equal(t, http.StatusNotFound, appErr.Status())
	require.Equal(t, "Không tìm thấy tài nguyên.", appErr.LocalizedMessage(LangVI))
	require.Equal(t, "The resource was not found.", appErr.LocalizedMessage(LangEN))
	require.False(t, appErr.Retryable())
	require.Equal(t, http.StatusNotFound, LookupDefinition(CodeNotFound).HTTPStatus)
}

func TestNamedErrorCreatesIndependentInstances(t *testing.T) {
	cause := stderrors.New("database lookup failed")
	first := ErrNotFound.WithCause(cause)
	second := ErrNotFound.WithCause(nil)
	first.Message = "custom message"
	first.Details = map[string]any{"resource": "first"}
	require.NotSame(t, first, second)
	require.Nil(t, second.Cause)
	require.Empty(t, second.Message)
	require.Empty(t, second.Details)
	require.Empty(t, From(ErrNotFound).Message)
	require.ErrorIs(t, first, cause)
}

func TestNamedErrorSupportsStandardMatching(t *testing.T) {
	wrapped := fmt.Errorf("lookup: %w", ErrNotFound.WithCause(stderrors.New("missing row")))
	require.ErrorIs(t, wrapped, ErrNotFound)
	require.NotErrorIs(t, wrapped, ErrConflict)
	require.ErrorIs(t, ErrNotFound, New(CodeNotFound))
	var appErr *AppError
	require.ErrorAs(t, fmt.Errorf("lookup: %w", ErrNotFound), &appErr)
	require.Equal(t, CodeNotFound, appErr.Code)
	require.ErrorIs(t, New(CodeNotFound), ErrNotFound)
}

func TestFromNormalizesWrappedNamedError(t *testing.T) {
	appErr := From(fmt.Errorf("provider call: %w", ErrProviderUnavailable))
	require.Equal(t, CodeProviderUnavailable, appErr.Code)
	require.True(t, appErr.Retryable())
	require.Equal(t, http.StatusServiceUnavailable, appErr.Status())
}

func TestNamedErrorOptionsKeepDefinitionUnchanged(t *testing.T) {
	custom := ErrValidationFailed.WithDetails(map[string]any{"field": "name"}).WithMessage("Name is required")
	require.Equal(t, "name", custom.Details["field"])
	require.Equal(t, "Name is required", custom.LocalizedMessage(LangVI))
	require.Equal(t, "The request data is invalid.", From(ErrValidationFailed).LocalizedMessage(LangEN))
}
