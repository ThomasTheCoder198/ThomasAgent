package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldError_NamesFieldAndCodeOnly(t *testing.T) {
	err := FieldError("apiKey", FieldCodeInvalid)
	require.Equal(t, CodeValidationFailed, err.Code)
	require.Equal(t, map[string]any{
		"fields": []map[string]string{{"field": "apiKey", "code": "INVALID"}},
	}, err.Details)
}
