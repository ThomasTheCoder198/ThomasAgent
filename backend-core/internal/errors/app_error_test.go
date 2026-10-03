package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_UsesCatalogStatusAndMessage(t *testing.T) {
	e := New(CodeNotFound)
	require.Equal(t, http.StatusNotFound, e.HTTPStatus())
	require.Equal(t, "Không tìm thấy tài nguyên.", e.LocalizedMessage(LangVI))
	require.Equal(t, "The resource was not found.", e.LocalizedMessage(LangEN))
}

func TestAppError_CustomMessageOverridesCatalog(t *testing.T) {
	e := New(CodeNotFound, WithMessage("Không tìm thấy KB"))
	require.Equal(t, "Không tìm thấy KB", e.LocalizedMessage(LangEN))
}

func TestToAppError_WrapsUnknownErrorsAsInternal(t *testing.T) {
	cause := stderrors.New("db exploded")
	e := ToAppError(cause)
	require.Equal(t, CodeInternalError, e.Code)
	require.ErrorIs(t, e, cause)
}

func TestToAppError_FindsWrappedAppError(t *testing.T) {
	inner := New(CodeConflict)
	e := ToAppError(fmt.Errorf("save: %w", inner))
	require.Same(t, inner, e)
}

func TestAppError_UnknownCodeFallsBackToInternalHTTPStatus(t *testing.T) {
	e := New(Code("NOT_IN_CATALOG"))
	require.Equal(t, http.StatusInternalServerError, e.HTTPStatus())
}

func TestLangFromHeader_SelectsLanguage(t *testing.T) {
	require.Equal(t, LangEN, LangFromHeader("en-US,en;q=0.9"))
	require.Equal(t, LangVI, LangFromHeader("vi-VN"))
	require.Equal(t, LangVI, LangFromHeader(""))
}
