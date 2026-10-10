package apptoken

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const pemTypePrivateKey = "PRIVATE KEY"

// LoadSigningKey never puts key material in its errors; callers log them.
func LoadSigningKey(cfg config.AppConfig, readFile func(string) ([]byte, error)) (ed25519.PrivateKey, error) {
	switch {
	case cfg.SigningKey != "" && cfg.SigningKeyFile != "":
		return nil, errors.ErrAppSigningKeyInvalid
	case cfg.SigningKey != "":
		return parseSigningKey([]byte(cfg.SigningKey))
	case cfg.SigningKeyFile != "":
		raw, err := readFile(cfg.SigningKeyFile)
		if err != nil {
			// The read error names the path and the OS reason, never the file's content.
			return nil, fmt.Errorf("%w: cannot read %s: %w", errors.ErrAppSigningKeyInvalid, cfg.SigningKeyFile, err)
		}
		return parseSigningKey(raw)
	default:
		return nil, errors.ErrAppSigningKeyMissing
	}
}

func parseSigningKey(raw []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != pemTypePrivateKey {
		return nil, errors.ErrAppSigningKeyInvalid
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.ErrAppSigningKeyInvalid
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.ErrAppSigningKeyInvalid
	}
	return key, nil
}
