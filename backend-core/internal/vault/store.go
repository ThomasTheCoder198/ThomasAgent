package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

type Store struct{ cipher *Cipher }

func NewStore(c *Cipher) *Store { return &Store{cipher: c} }

func (s *Store) Put(ctx context.Context, db postgres.DBTX, plaintext string) (uuid.UUID, error) {
	id := uuid.New()
	sealed, err := s.cipher.Seal([]byte(plaintext), id[:])
	if err != nil {
		return uuid.Nil, err
	}
	_, err = db.Exec(ctx, `INSERT INTO secrets (id, key_id, nonce, ciphertext) VALUES ($1, $2, $3, $4)`,
		id, sealed.KeyID, sealed.Nonce, sealed.Ciphertext)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert secret: %w", err)
	}
	return id, nil
}

func (s *Store) Replace(ctx context.Context, db postgres.DBTX, id uuid.UUID, plaintext string) error {
	sealed, err := s.cipher.Seal([]byte(plaintext), id[:])
	if err != nil {
		return err
	}
	command, err := db.Exec(ctx, `UPDATE secrets SET key_id = $2, nonce = $3, ciphertext = $4, updated_at = now() WHERE id = $1`,
		id, sealed.KeyID, sealed.Nonce, sealed.Ciphertext)
	if err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrSecretNotFound
	}
	return nil
}

func (s *Store) Get(ctx context.Context, db postgres.DBTX, id uuid.UUID) (string, error) {
	var sealed Sealed
	err := db.QueryRow(ctx, `SELECT key_id, nonce, ciphertext FROM secrets WHERE id = $1`, id).
		Scan(&sealed.KeyID, &sealed.Nonce, &sealed.Ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load secret: %w", ErrSecretNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("load secret: %w", err)
	}
	plain, err := s.cipher.Open(sealed, id[:])
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Store) Delete(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	if _, err := db.Exec(ctx, `DELETE FROM secrets WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}
