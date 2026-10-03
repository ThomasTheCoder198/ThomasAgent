package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUsesCatalogStatusAndMessage(t *testing.T) {
	e := New(CodeNotFound)
	require.Equal(t, http.StatusNotFound, e.Status())
	require.Equal(t, "Không tìm thấy tài nguyên.", e.LocalizedMessage(LangVI))
	require.Equal(t, "The resource was not found.", e.LocalizedMessage(LangEN))
}

func TestCustomMessageOverridesCatalog(t *testing.T) {
	e := New(CodeNotFound, WithMessage("Không tìm thấy KB"))
	require.Equal(t, "Không tìm thấy KB", e.LocalizedMessage(LangEN))
}

func TestFromWrapsUnknownErrorsAsInternal(t *testing.T) {
	cause := errors.New("db exploded")
	e := From(cause)
	require.Equal(t, CodeInternalError, e.Code)
	require.ErrorIs(t, e, cause)
}

func TestFromFindsWrappedAppError(t *testing.T) {
	inner := New(CodeConflict)
	e := From(fmt.Errorf("save: %w", inner))
	require.Same(t, inner, e)
}

func TestUnknownCodeFallsBackToInternalStatus(t *testing.T) {
	e := New(Code("NOT_IN_CATALOG"))
	require.Equal(t, http.StatusInternalServerError, e.Status())
}

func TestLangFromHeader(t *testing.T) {
	require.Equal(t, LangEN, LangFromHeader("en-US,en;q=0.9"))
	require.Equal(t, LangVI, LangFromHeader("vi-VN"))
	require.Equal(t, LangVI, LangFromHeader(""))
}
