package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type decodeTarget struct {
	Name   string `json:"name"`
	APIKey int    `json:"apiKey"`
}

func decodeBody(t *testing.T, body string, limit int64) (decodeTarget, error) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(body))
	var target decodeTarget
	err := DecodeJSON(httptest.NewRecorder(), req, &target, limit)
	return target, err
}

func TestDecodeJSON_AcceptsOneValue(t *testing.T) {
	target, err := decodeBody(t, `{"name":"a"}`, 64)
	require.NoError(t, err)
	require.Equal(t, "a", target.Name)
}

func TestDecodeJSON_RejectsBadInput(t *testing.T) {
	tests := map[string]string{
		"malformed":        `{"name":`,
		"empty":            ``,
		"wrong type":       `{"apiKey":"sk-secret-value"}`,
		"trailing value":   `{"name":"a"}{"name":"b"}`,
		"trailing garbage": `{"name":"a"} x`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeBody(t, body, 64)
			appErr := errors.ToAppError(err)
			require.Equal(t, errors.CodeValidationFailed, appErr.Code)
			require.Equal(t, errors.FieldError("body", errors.FieldCodeInvalid).Details, appErr.Details)
		})
	}
}

func TestDecodeJSON_RejectsOversizeBody(t *testing.T) {
	const limit = 16
	_, err := decodeBody(t, `{"name":"`+strings.Repeat("a", limit)+`"}`, limit)
	require.Equal(t, errors.CodePayloadTooLarge, errors.ToAppError(err).Code)
}

func TestDecodeJSON_ErrorResponseNeverEchoesBody(t *testing.T) {
	_, err := decodeBody(t, `{"apiKey":"sk-secret-value"}`, 64)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	WriteError(rec, req, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, rec.Body.String(), "sk-secret-value")
}
