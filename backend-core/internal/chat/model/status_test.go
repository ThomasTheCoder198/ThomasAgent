package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestStatusLabel_FollowsTheLanguageAndKnowsEveryKey(t *testing.T) {
	require.Equal(t, "Searching the KB…", StatusLabel(StatusSearchingKB, errors.LangEN))
	require.Equal(t, "Đang tìm trong KB…", StatusLabel(StatusSearchingKB, errors.LangVI))
	require.Equal(t, "Đang tìm trong KB…", StatusLabel(StatusSearchingKB, errors.Lang("")), "an unset language is Vietnamese")
	require.Empty(t, StatusLabel(StatusKey("nope"), errors.LangEN), "an unknown key has no label")
	for key := range statusLabels {
		require.NotEmpty(t, StatusLabel(key, errors.LangVI), key)
		require.NotEmpty(t, StatusLabel(key, errors.LangEN), key)
	}
}
