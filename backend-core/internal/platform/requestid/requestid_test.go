package requestid_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

type sharedDefinition struct {
	Header        string   `json:"header"`
	Pattern       string   `json:"pattern"`
	RequestPrefix string   `json:"requestPrefix"`
	JobPrefix     string   `json:"jobPrefix"`
	Valid         []string `json:"valid"`
	Invalid       []string `json:"invalid"`
}

func loadDefinition(t *testing.T) sharedDefinition {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "request-id.json"))
	require.NoError(t, err)
	var def sharedDefinition
	require.NoError(t, json.Unmarshal(raw, &def))
	return def
}

func TestContract_MatchesSharedDefinition(t *testing.T) {
	def := loadDefinition(t)
	require.Equal(t, def.Header, requestid.Header)
	require.Equal(t, def.Pattern, requestid.Pattern)
	require.Equal(t, def.RequestPrefix, requestid.RequestPrefix)
	require.Equal(t, def.JobPrefix, requestid.JobPrefix)
}

func TestIsValid_FollowsSharedExamples(t *testing.T) {
	def := loadDefinition(t)
	for _, id := range def.Valid {
		require.True(t, requestid.IsValid(id), id)
	}
	for _, id := range def.Invalid {
		require.False(t, requestid.IsValid(id), id)
	}
}

func TestNew_ProducesValidPrefixedIDs(t *testing.T) {
	request, job := requestid.NewRequest(), requestid.NewJob()
	require.True(t, requestid.IsValid(request))
	require.True(t, requestid.IsValid(job))
	require.Equal(t, requestid.RequestPrefix, request[:len(requestid.RequestPrefix)])
	require.Equal(t, requestid.JobPrefix, job[:len(requestid.JobPrefix)])
	require.NotEqual(t, request[len(requestid.RequestPrefix):], requestid.NewRequest()[len(requestid.RequestPrefix):])
}
