package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestNormalizeName_TrimsAndBounds(t *testing.T) {
	name, err := NormalizeName("  Công ty Acme ")
	require.NoError(t, err)
	require.Equal(t, "Công ty Acme", name)
	for raw, code := range map[string]string{
		"   ": errors.FieldCodeRequired, strings.Repeat("ư", NameMaxLength+1): errors.FieldCodeInvalid, "Acme\x00": errors.FieldCodeInvalid,
	} {
		_, err := NormalizeName(raw)
		require.ErrorIs(t, err, errors.ErrValidationFailed)
		require.Equal(t, []map[string]string{{"field": "name", "code": code}}, errors.ToAppError(err).Details["fields"])
	}
	_, err = NormalizeName(strings.Repeat("ư", NameMaxLength))
	require.NoError(t, err, "the limit counts characters, not bytes")
}
