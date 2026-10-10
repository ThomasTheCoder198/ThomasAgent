package apptoken

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

var errUnreadable = stderrors.New("no such file")

func pkcs8PEM(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestLoadSigningKey_ReadsPKCS8FromValueOrFile(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	encoded := pkcs8PEM(t, key)
	fromValue, err := LoadSigningKey(config.AppConfig{SigningKey: encoded}, nil)
	require.NoError(t, err)
	require.Equal(t, key, fromValue)
	fromFile, err := LoadSigningKey(config.AppConfig{SigningKeyFile: "/run/secrets/key.pem"}, func(path string) ([]byte, error) {
		require.Equal(t, "/run/secrets/key.pem", path)
		return []byte(encoded), nil
	})
	require.NoError(t, err)
	require.Equal(t, key, fromFile)
}

func TestLoadSigningKey_RejectsMissingAmbiguousOrWrongKeys(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	unreadable := func(string) ([]byte, error) { return nil, errUnreadable }
	_, err = LoadSigningKey(config.AppConfig{}, nil)
	require.ErrorIs(t, err, errors.ErrAppSigningKeyMissing)
	_, err = LoadSigningKey(config.AppConfig{SigningKey: "x", SigningKeyFile: "y"}, nil)
	require.ErrorIs(t, err, errors.ErrAppSigningKeyInvalid)
	_, err = LoadSigningKey(config.AppConfig{SigningKey: "not a pem"}, nil)
	require.ErrorIs(t, err, errors.ErrAppSigningKeyInvalid)
	_, err = LoadSigningKey(config.AppConfig{SigningKey: pkcs8PEM(t, ecKey)}, nil)
	require.ErrorIs(t, err, errors.ErrAppSigningKeyInvalid)
	_, err = LoadSigningKey(config.AppConfig{SigningKeyFile: "missing.pem"}, unreadable)
	require.ErrorIs(t, err, errors.ErrAppSigningKeyInvalid)
	require.ErrorIs(t, err, errUnreadable, "the read error stays in the chain")
	require.ErrorContains(t, err, "missing.pem")
	require.NotContains(t, err.Error(), "PRIVATE KEY")
}
