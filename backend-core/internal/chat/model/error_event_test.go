package model

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestErrorEvent_KeepsPublicCodesAndMasksTheRest(t *testing.T) {
	for _, code := range []errors.Code{
		errors.CodeProviderUnavailable, errors.CodeRagxUnavailable, errors.CodeDependencyTimeout, errors.CodeRunDeadlineExceeded,
		errors.CodeRunInterrupted, errors.CodeRunStreamIncomplete, // worker and relay endings of a run
		errors.CodeRunOutputTooLarge, errors.CodeValidationFailed, // 4xx codes are never masked
	} {
		got := ErrorEvent(errors.New(code), errors.LangVI)
		require.Equal(t, string(code), got.Code)
		require.NotEmpty(t, got.Message)
	}
	masked := ErrorEvent(stderrors.New("connection to 10.0.0.5 refused"), errors.LangVI)
	require.Equal(t, string(errors.CodeInternalError), masked.Code)
	require.NotContains(t, masked.Message, "10.0.0.5")
}
