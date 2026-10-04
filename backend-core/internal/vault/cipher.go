package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const masterKeyBytes = 32

var (
	ErrInvalidMasterKey = errors.New("vault: master key must be base64 of exactly 32 bytes")
	ErrDecrypt          = errors.New("vault: cannot decrypt secret")
)

type Sealed struct {
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
}

type Cipher struct {
	keyID string
	aead  cipher.AEAD
}

func NewCipher(keyID, masterKeyBase64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil || len(key) != masterKeyBytes {
		return nil, ErrInvalidMasterKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault gcm: %w", err)
	}
	return &Cipher{keyID: keyID, aead: aead}, nil
}

func (c *Cipher) Seal(plaintext, aad []byte) (Sealed, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, fmt.Errorf("vault nonce: %w", err)
	}
	return Sealed{KeyID: c.keyID, Nonce: nonce, Ciphertext: c.aead.Seal(nil, nonce, plaintext, aad)}, nil
}

func (c *Cipher) Open(s Sealed, aad []byte) ([]byte, error) {
	if len(s.Nonce) != c.aead.NonceSize() {
		return nil, ErrDecrypt
	}
	plain, err := c.aead.Open(nil, s.Nonce, s.Ciphertext, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}
