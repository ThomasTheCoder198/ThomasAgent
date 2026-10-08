package apptoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const vectorPath = "../../app-token.json"

func TestRender_MatchesCommittedVector(t *testing.T) {
	rendered, err := Render()
	require.NoError(t, err)
	committed, err := os.ReadFile(vectorPath)
	require.NoError(t, err, "run task gen to create app-token.json")
	require.Equal(t, string(committed), string(rendered), "app-token.json is stale: run task gen")
}

func TestBuild_ValidTokensVerifyWithTrustedKey(t *testing.T) {
	vector, err := Build()
	require.NoError(t, err)
	require.Len(t, vector.Keys, 1)
	publicKey, err := base64.StdEncoding.DecodeString(vector.Keys[0].PublicKey)
	require.NoError(t, err)
	for _, valid := range vector.Valid {
		parts := strings.Split(valid.Token, ".")
		require.Len(t, parts, 3, valid.Name)
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		require.NoError(t, err, valid.Name)
		require.True(t, ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), signature), valid.Name)
	}
}

func TestBuild_EveryInvalidCaseNamesOneReason(t *testing.T) {
	vector, err := Build()
	require.NoError(t, err)
	reasons := map[string]bool{}
	for _, invalid := range vector.Invalid {
		require.NotEmpty(t, invalid.Reason, invalid.Name)
		reasons[invalid.Reason] = true
	}
	for _, reason := range []string{
		ReasonMalformed, ReasonWrongAlgorithm, ReasonWrongType, ReasonUnknownKid, ReasonBadSignature,
		ReasonWrongIssuer, ReasonWrongAudience, ReasonMissingClaim, ReasonInvalidClaim, ReasonExpired,
		ReasonNotYetValid, ReasonLifetimeTooLong,
	} {
		require.True(t, reasons[reason], "no invalid case for %s", reason)
	}
}
