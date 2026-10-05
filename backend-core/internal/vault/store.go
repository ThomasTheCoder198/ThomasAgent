package vault

import (
	"context"
	"encoding/binary"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type Store struct{ cipher *Cipher }

const (
	legacyAADVersion     = 1
	boundAADVersion      = 2
	structuredAADVersion = 3
	boundAADSeparator    = ":"
)

func NewStore(c *Cipher) *Store { return &Store{cipher: c} }

func (s *Store) Put(ctx context.Context, db postgres.DBTX, plaintext string) (uuid.UUID, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	sealed, err := s.cipher.Seal([]byte(plaintext), secretAAD(tenantID, id, s.cipher.keyID))
	if err != nil {
		return uuid.Nil, err
	}
	_, err = db.Exec(ctx, `INSERT INTO secrets (tenant_id, id, key_id, nonce, ciphertext, aad_version) VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, id, sealed.KeyID, sealed.Nonce, sealed.Ciphertext, structuredAADVersion)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert secret: %w", err)
	}
	return id, nil
}

func (s *Store) Replace(ctx context.Context, db postgres.DBTX, id uuid.UUID, plaintext string) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	sealed, err := s.cipher.Seal([]byte(plaintext), secretAAD(tenantID, id, s.cipher.keyID))
	if err != nil {
		return err
	}
	command, err := db.Exec(ctx, `UPDATE secrets SET key_id = $3, nonce = $4, ciphertext = $5, aad_version = $6, updated_at = now() WHERE id = $1 AND tenant_id = $2`,
		id, tenantID, sealed.KeyID, sealed.Nonce, sealed.Ciphertext, structuredAADVersion)
	if err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrSecretNotFound
	}
	return nil
}

func (s *Store) Get(ctx context.Context, db postgres.DBTX, id uuid.UUID) (string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return "", err
	}
	var sealed Sealed
	var version int
	err = db.QueryRow(ctx, `SELECT key_id, nonce, ciphertext, aad_version FROM secrets WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&sealed.KeyID, &sealed.Nonce, &sealed.Ciphertext, &version)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load secret: %w", ErrSecretNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("load secret: %w", err)
	}
	aad, err := aadForVersion(version, tenantID, id, sealed.KeyID)
	if err != nil {
		return "", err
	}
	plain, err := s.cipher.Open(sealed, aad)
	if err != nil {
		return "", err
	}
	if version != structuredAADVersion {
		upgraded, err := s.cipher.Seal(plain, secretAAD(tenantID, id, sealed.KeyID))
		if err != nil {
			return "", err
		}
		// Compare the old envelope so a concurrent key replacement cannot be overwritten.
		_, err = db.Exec(ctx, `UPDATE secrets SET nonce=$3, ciphertext=$4, aad_version=$5, updated_at=now()
			WHERE id=$1 AND tenant_id=$2 AND aad_version=$6 AND nonce=$7 AND ciphertext=$8`,
			id, tenantID, upgraded.Nonce, upgraded.Ciphertext, structuredAADVersion, version, sealed.Nonce, sealed.Ciphertext)
		if err != nil {
			return "", fmt.Errorf("upgrade secret AAD: %w", err)
		}
	}
	return string(plain), nil
}

func aadForVersion(version int, tenantID string, id uuid.UUID, keyID string) ([]byte, error) {
	switch version {
	case legacyAADVersion:
		return id[:], nil
	case boundAADVersion:
		return boundSecretAAD(tenantID, id, keyID), nil
	case structuredAADVersion:
		return secretAAD(tenantID, id, keyID), nil
	default:
		return nil, ErrDecrypt
	}
}

func secretAAD(tenantID string, id uuid.UUID, keyID string) []byte {
	var aad []byte
	for _, field := range []string{tenantID, id.String(), keyID} {
		aad = binary.AppendUvarint(aad, uint64(len(field)))
		aad = append(aad, field...)
	}
	return aad
}

func boundSecretAAD(tenantID string, id uuid.UUID, keyID string) []byte {
	return []byte(tenantID + boundAADSeparator + id.String() + boundAADSeparator + keyID)
}

func (s *Store) Delete(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	command, err := db.Exec(ctx, `DELETE FROM secrets WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrSecretNotFound
	}
	return nil
}
