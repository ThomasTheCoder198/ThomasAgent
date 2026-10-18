package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 10, 17, 8, 30, 0, 123456789, time.UTC)
	id := uuid.New()
	gotAt, gotID, err := decodeCursor(encodeCursor(at, id))
	require.NoError(t, err)
	require.True(t, at.Equal(gotAt))
	require.Equal(t, id, gotID)
}

func TestCursorRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"!!", "bm8tc2VwYXJhdG9y", "MjAyNnx4"} {
		_, _, err := decodeCursor(bad)
		require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err), bad)
	}
}
