package apptoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const vectorPath = "../../../../contracts/app-token.json"

func TestToolScope(t *testing.T) {
	require.Equal(t, Scope("tool:kb_search"), ToolScope("kb_search"))
}

type vectorFile struct {
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
	Keys     []struct {
		Kid            string `json:"kid"`
		PublicKey      string `json:"publicKey"`
		PrivateKeySeed string `json:"privateKeySeed"`
	} `json:"keys"`
	Valid []struct {
		Name   string         `json:"name"`
		Token  string         `json:"token"`
		Claims map[string]any `json:"claims"`
	} `json:"valid"`
}

func loadVector(t *testing.T) vectorFile {
	t.Helper()
	raw, err := os.ReadFile(vectorPath)
	require.NoError(t, err)
	var v vectorFile
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

func decodeSegment(t *testing.T, segment string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func claimString(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return value
}

// The vector's header is alg,typ,kid; golang-jwt writes header keys sorted, so headers are compared as JSON objects.
// The payload follows the vector's field order exactly and must match byte for byte.
func TestSign_MatchesTheSharedVector(t *testing.T) {
	vector := loadVector(t)
	seed, err := base64.StdEncoding.DecodeString(vector.Keys[0].PrivateKeySeed)
	require.NoError(t, err)
	publicKey, err := base64.StdEncoding.DecodeString(vector.Keys[0].PublicKey)
	require.NoError(t, err)
	for _, valid := range vector.Valid {
		t.Run(valid.Name, func(t *testing.T) {
			iat := int64(valid.Claims["iat"].(float64))
			exp := int64(valid.Claims["exp"].(float64))
			cfg := config.AppConfig{
				SigningKeyID: vector.Keys[0].Kid, TokenIssuer: vector.Issuer, TokenAudience: vector.Audience,
				TokenTTL: time.Duration(exp-iat) * time.Second,
			}
			signer := NewAppTokenSigner(ed25519.NewKeyFromSeed(seed), cfg,
				func() time.Time { return time.Unix(iat, 0) }, func() string { return claimString(valid.Claims, "jti") })
			token, err := signer.Sign(Claims{
				OrgID: claimString(valid.Claims, "oid"), KnowledgeBaseID: claimString(valid.Claims, "kb"),
				Scope: Scope(claimString(valid.Claims, "scp")), RequestID: claimString(valid.Claims, "rid"),
			})
			require.NoError(t, err)
			got, want := strings.Split(token, "."), strings.Split(valid.Token, ".")
			require.Len(t, got, 3)
			require.Equal(t, decodeSegment(t, want[0]), decodeSegment(t, got[0]))
			require.Equal(t, want[1], got[1], "payload bytes must match the vector")
			signature, err := base64.RawURLEncoding.DecodeString(got[2])
			require.NoError(t, err)
			require.True(t, ed25519.Verify(publicKey, []byte(got[0]+"."+got[1]), signature))
		})
	}
}

func TestSign_RejectsMissingOrgScopeOrRequestID(t *testing.T) {
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := NewAppTokenSigner(key, config.AppConfig{SigningKeyID: "k", TokenIssuer: "i", TokenAudience: "a", TokenTTL: time.Minute},
		time.Now, func() string { return "jti" })
	valid := Claims{OrgID: "org-1", Scope: ScopeKBRead, RequestID: "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"}
	for name, claims := range map[string]Claims{
		"no org":         {Scope: valid.Scope, RequestID: valid.RequestID},
		"platform org":   {OrgID: "default", Scope: valid.Scope, RequestID: valid.RequestID},
		"no scope":       {OrgID: valid.OrgID, RequestID: valid.RequestID},
		"no request id":  {OrgID: valid.OrgID, Scope: valid.Scope},
		"bad request id": {OrgID: valid.OrgID, Scope: valid.Scope, RequestID: "forged\nid"},
	} {
		_, err := signer.Sign(claims)
		require.ErrorIs(t, err, errors.ErrAppTokenClaimsInvalid, name)
	}
	_, err = signer.Sign(valid)
	require.NoError(t, err)
}

func TestIssue_ReturnsTheTokenAndTheJTIItSigned(t *testing.T) {
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := NewAppTokenSigner(key, config.AppConfig{SigningKeyID: "k", TokenIssuer: "i", TokenAudience: "a", TokenTTL: time.Minute},
		time.Now, func() string { return "jti-issued" })
	claims := Claims{OrgID: "org-a", Scope: ScopeKBRead, RequestID: "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"}
	issued, err := signer.Issue(claims)
	require.NoError(t, err)
	require.Equal(t, "jti-issued", issued.ID)
	require.Equal(t, "jti-issued", claimString(decodeSegment(t, strings.Split(issued.Token, ".")[1]), "jti"))
	token, err := signer.Sign(claims)
	require.NoError(t, err)
	require.Len(t, strings.Split(token, "."), 3)
}
