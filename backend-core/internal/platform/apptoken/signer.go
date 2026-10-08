package apptoken

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type Scope string

const (
	ScopeOrgProvision Scope = "org:provision"
	ScopeKBRead       Scope = "kb:read"
	ScopeKBWrite      Scope = "kb:write"

	TokenType   = "thomas-app+jwt"
	headerType  = "typ"
	headerKeyID = "kid"
)

// Claims are what a caller decides; issuer, audience, times and jti come from the signer.
type Claims struct {
	OrgID           string
	KnowledgeBaseID string
	Scope           Scope
	RequestID       string
}

// payload keeps the field order of contracts/app-token.json and a single-string aud (jwt.ClaimStrings would
// serialize an array by default), so the encoded payload is byte-identical to the shared vector.
type payload struct {
	Iss string `json:"iss"`
	Aud string `json:"aud"`
	Oid string `json:"oid"`
	Kb  string `json:"kb,omitempty"`
	Scp string `json:"scp"`
	Rid string `json:"rid"`
	Jti string `json:"jti"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func (p payload) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(p.Exp, 0)), nil
}
func (p payload) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(p.Iat, 0)), nil
}
func (p payload) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (p payload) GetIssuer() (string, error)              { return p.Iss, nil }
func (p payload) GetSubject() (string, error)             { return "", nil }
func (p payload) GetAudience() (jwt.ClaimStrings, error)  { return jwt.ClaimStrings{p.Aud}, nil }

type Signer struct {
	key    ed25519.PrivateKey
	cfg    config.AppConfig
	now    func() time.Time
	newJTI func() string
}

func NewAppTokenSigner(key ed25519.PrivateKey, cfg config.AppConfig, now func() time.Time, newJTI func() string) *Signer {
	return &Signer{key: key, cfg: cfg, now: now, newJTI: newJTI}
}

func (s *Signer) Sign(c Claims) (string, error) {
	if c.OrgID == "" || c.OrgID == tenant.PlatformID || c.Scope == "" || !requestid.IsValid(c.RequestID) {
		return "", errors.ErrInternalError.WithCause(errors.ErrAppTokenClaimsInvalid)
	}
	issued := s.now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, payload{
		Iss: s.cfg.TokenIssuer, Aud: s.cfg.TokenAudience, Oid: c.OrgID, Kb: c.KnowledgeBaseID, Scp: string(c.Scope),
		Rid: c.RequestID, Jti: s.newJTI(), Iat: issued, Exp: issued + int64(s.cfg.TokenTTL/time.Second),
	})
	token.Header[headerType] = TokenType
	token.Header[headerKeyID] = s.cfg.SigningKeyID
	signed, err := token.SignedString(s.key)
	if err != nil {
		return "", errors.ErrInternalError.WithCause(fmt.Errorf("sign app token: %w", err))
	}
	return signed, nil
}
