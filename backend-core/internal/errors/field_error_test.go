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

func TestFieldConflict_NamesFieldAndTakenCode(t *testing.T) {
	err := FieldConflict("name")
	require.Equal(t, CodeConflict, err.Code)
	require.Equal(t, []map[string]string{{"field": "name", "code": FieldCodeTaken}}, err.Details["fields"])
}
