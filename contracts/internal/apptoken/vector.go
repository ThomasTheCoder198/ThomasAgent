package apptoken

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Fixed values make the vector byte-for-byte reproducible: Ed25519 signatures are deterministic.
const (
	Algorithm                = "EdDSA"
	TokenType                = "thomas-app+jwt"
	Issuer                   = "thomas-core"
	Audience                 = "ragx"
	LeewaySeconds            = 10
	MaxLifetimeSeconds       = 60
	Now                int64 = 1791000000
	TrustedKeyID             = "test-1"
	UntrustedKeyID           = "test-2"
	seedLabel                = "thomas-app-token-test-key-"
	OrgID                    = "org-alpha"
	KnowledgeBaseID          = "8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f"
	RequestID                = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	baseJTI                  = "jti-0001"
	issuedBefore       int64 = 5
	expiresAfter       int64 = 55
	futureOffset       int64 = 5
	tooLongExpiry      int64 = 120
	ScopeProvision           = "org:provision"
	ScopeRead                = "kb:read"
	ScopeTool                = "tool:kb_search"
	jsonIndent               = "  "
)

const (
	ReasonMalformed       = "malformed"
	ReasonWrongAlgorithm  = "wrong_algorithm"
	ReasonWrongType       = "wrong_type"
	ReasonUnknownKid      = "unknown_kid"
	ReasonBadSignature    = "bad_signature"
	ReasonWrongIssuer     = "wrong_issuer"
	ReasonWrongAudience   = "wrong_audience"
	ReasonMissingClaim    = "missing_claim"
	ReasonInvalidClaim    = "invalid_claim"
	ReasonExpired         = "expired"
	ReasonNotYetValid     = "not_yet_valid"
	ReasonLifetimeTooLong = "lifetime_too_long"
)

type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

type Claims struct {
	Iss string `json:"iss,omitempty"`
	Aud string `json:"aud,omitempty"`
	Oid string `json:"oid,omitempty"`
	Kb  string `json:"kb,omitempty"`
	Scp string `json:"scp,omitempty"`
	Rid string `json:"rid,omitempty"`
	Jti string `json:"jti,omitempty"`
	Iat int64  `json:"iat,omitempty"`
	Exp int64  `json:"exp,omitempty"`
}

type Key struct {
	Kid            string `json:"kid"`
	PublicKey      string `json:"publicKey"`
	PrivateKeySeed string `json:"privateKeySeed"`
}

type ValidCase struct {
	Name   string `json:"name"`
	Token  string `json:"token"`
	Claims Claims `json:"claims"`
}

type InvalidCase struct {
	Name   string `json:"name"`
	Token  string `json:"token"`
	Reason string `json:"reason"`
}

type Vector struct {
	Algorithm          string        `json:"algorithm"`
	Type               string        `json:"type"`
	Issuer             string        `json:"issuer"`
	Audience           string        `json:"audience"`
	LeewaySeconds      int           `json:"leewaySeconds"`
	MaxLifetimeSeconds int           `json:"maxLifetimeSeconds"`
	Now                int64         `json:"now"`
	Keys               []Key         `json:"keys"`
	Valid              []ValidCase   `json:"valid"`
	Invalid            []InvalidCase `json:"invalid"`
}

type signingKey struct {
	kid     string
	private ed25519.PrivateKey
	seed    []byte
}

func keyFor(kid string) signingKey {
	seed := sha256.Sum256([]byte(seedLabel + kid))
	return signingKey{kid: kid, private: ed25519.NewKeyFromSeed(seed[:]), seed: seed[:]}
}

func segment(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal token segment: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sign(key signingKey, header Header, claims Claims) (string, error) {
	head, err := segment(header)
	if err != nil {
		return "", err
	}
	body, err := segment(claims)
	if err != nil {
		return "", err
	}
	input := head + "." + body
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key.private, []byte(input))), nil
}

func header(kid string) Header {
	return Header{Alg: Algorithm, Typ: TokenType, Kid: kid}
}

func baseClaims(scope string) Claims {
	return Claims{
		Iss: Issuer, Aud: Audience, Oid: OrgID, Scp: scope, Rid: RequestID, Jti: baseJTI,
		Iat: Now - issuedBefore, Exp: Now + expiresAfter,
	}
}

type claimEdit func(*Claims)

func edited(scope string, edit claimEdit) Claims {
	claims := baseClaims(scope)
	edit(&claims)
	return claims
}

func validCases(trusted signingKey) ([]ValidCase, error) {
	inputs := []struct {
		name   string
		claims Claims
	}{
		{"provision", baseClaims(ScopeProvision)},
		{"kb read", baseClaims(ScopeRead)},
		{"tool call carries kb", edited(ScopeTool, func(c *Claims) { c.Kb = KnowledgeBaseID })},
		{"expired inside leeway", edited(ScopeRead, func(c *Claims) { c.Exp = Now - LeewaySeconds + 1; c.Iat = c.Exp - MaxLifetimeSeconds })},
	}
	cases := make([]ValidCase, 0, len(inputs))
	for _, in := range inputs {
		token, err := sign(trusted, header(trusted.kid), in.claims)
		if err != nil {
			return nil, err
		}
		cases = append(cases, ValidCase{Name: in.name, Token: token, Claims: in.claims})
	}
	return cases, nil
}

type invalidInput struct {
	name   string
	key    signingKey
	header Header
	claims Claims
	reason string
}

func invalidInputs(trusted, untrusted signingKey) []invalidInput {
	good := header(trusted.kid)
	return []invalidInput{
		{"expired", trusted, good, edited(ScopeRead, func(c *Claims) { c.Exp = Now - LeewaySeconds - 1; c.Iat = c.Exp - MaxLifetimeSeconds }), ReasonExpired},
		{"issued in the future", trusted, good, edited(ScopeRead, func(c *Claims) { c.Iat = Now + LeewaySeconds + futureOffset; c.Exp = c.Iat + MaxLifetimeSeconds }), ReasonNotYetValid},
		{"lifetime too long", trusted, good, edited(ScopeRead, func(c *Claims) { c.Exp = Now + tooLongExpiry }), ReasonLifetimeTooLong},
		{"unknown kid", untrusted, header(untrusted.kid), baseClaims(ScopeRead), ReasonUnknownKid},
		{"other key under trusted kid", untrusted, good, baseClaims(ScopeRead), ReasonBadSignature},
		{"wrong audience", trusted, good, edited(ScopeRead, func(c *Claims) { c.Aud = "other-app" }), ReasonWrongAudience},
		{"wrong issuer", trusted, good, edited(ScopeRead, func(c *Claims) { c.Iss = "someone-else" }), ReasonWrongIssuer},
		{"wrong type", trusted, Header{Alg: Algorithm, Typ: "JWT", Kid: trusted.kid}, baseClaims(ScopeRead), ReasonWrongType},
		{"missing org", trusted, good, edited(ScopeRead, func(c *Claims) { c.Oid = "" }), ReasonMissingClaim},
		{"missing scope", trusted, good, edited(ScopeRead, func(c *Claims) { c.Scp = "" }), ReasonMissingClaim},
		{"missing jti", trusted, good, edited(ScopeRead, func(c *Claims) { c.Jti = "" }), ReasonMissingClaim},
		{"org id with path characters", trusted, good, edited(ScopeRead, func(c *Claims) { c.Oid = "../org" }), ReasonInvalidClaim},
		{"forged request id", trusted, good, edited(ScopeRead, func(c *Claims) { c.Rid = "forged id" }), ReasonInvalidClaim},
	}
}

func invalidCases(trusted, untrusted signingKey) ([]InvalidCase, error) {
	inputs := invalidInputs(trusted, untrusted)
	cases := make([]InvalidCase, 0, len(inputs)+2)
	for _, in := range inputs {
		token, err := sign(in.key, in.header, in.claims)
		if err != nil {
			return nil, err
		}
		cases = append(cases, InvalidCase{Name: in.name, Token: token, Reason: in.reason})
	}
	unsigned, err := unsignedToken(trusted.kid)
	if err != nil {
		return nil, err
	}
	cases = append(cases,
		InvalidCase{Name: "alg none", Token: unsigned, Reason: ReasonWrongAlgorithm},
		InvalidCase{Name: "not a jwt", Token: "not-a-token", Reason: ReasonMalformed},
	)
	return cases, nil
}

func unsignedToken(kid string) (string, error) {
	head, err := segment(Header{Alg: "none", Typ: TokenType, Kid: kid})
	if err != nil {
		return "", err
	}
	body, err := segment(baseClaims(ScopeRead))
	if err != nil {
		return "", err
	}
	return head + "." + body + ".", nil
}

func Build() (Vector, error) {
	trusted, untrusted := keyFor(TrustedKeyID), keyFor(UntrustedKeyID)
	valid, err := validCases(trusted)
	if err != nil {
		return Vector{}, err
	}
	invalid, err := invalidCases(trusted, untrusted)
	if err != nil {
		return Vector{}, err
	}
	public, ok := trusted.private.Public().(ed25519.PublicKey)
	if !ok {
		return Vector{}, fmt.Errorf("unexpected public key type")
	}
	return Vector{
		Algorithm: Algorithm, Type: TokenType, Issuer: Issuer, Audience: Audience,
		LeewaySeconds: LeewaySeconds, MaxLifetimeSeconds: MaxLifetimeSeconds, Now: Now,
		Keys: []Key{{
			Kid: trusted.kid, PublicKey: base64.StdEncoding.EncodeToString(public),
			PrivateKeySeed: base64.StdEncoding.EncodeToString(trusted.seed),
		}},
		Valid: valid, Invalid: invalid,
	}, nil
}

// Render is the exact file content of contracts/app-token.json (two-space indent, trailing newline).
func Render() ([]byte, error) {
	vector, err := Build()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", jsonIndent)
	if err := encoder.Encode(vector); err != nil {
		return nil, fmt.Errorf("encode app-token vector: %w", err)
	}
	return out.Bytes(), nil
}
