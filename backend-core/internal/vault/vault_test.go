package vault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

func randomKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, masterKeyBytes)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

func TestNewCipherRejectsBadMasterKey(t *testing.T) {
	for name, key := range map[string]string{
		"empty":     "",
		"not b64":   "%%%",
		"too long":  base64.StdEncoding.EncodeToString(make([]byte, 33)),
		"too short": base64.StdEncoding.EncodeToString([]byte("short")),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewCipher("v1", key)
			require.ErrorIs(t, err, ErrInvalidMasterKey)
		})
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	sealed, err := c.Seal([]byte("sk-or-v1-secret"), []byte("row-1"))
	require.NoError(t, err)
	require.NotContains(t, string(sealed.Ciphertext), "sk-or")
	plain, err := c.Open(sealed, []byte("row-1"))
	require.NoError(t, err)
	require.Equal(t, "sk-or-v1-secret", string(plain))
}

func TestOpenFailsWithWrongAADOrKey(t *testing.T) {
	c1, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	c2, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	sealed, err := c1.Seal([]byte("secret"), []byte("row-1"))
	require.NoError(t, err)
	_, err = c1.Open(sealed, []byte("row-2"))
	require.ErrorIs(t, err, ErrDecrypt)
	_, err = c2.Open(sealed, []byte("row-1"))
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStorePersistsEncrypted(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := context.Background()
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	s := NewStore(c)

	id, err := s.Put(ctx, pool, "sk-first")
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, pool.QueryRow(ctx, "SELECT ciphertext FROM secrets WHERE id = $1", id).Scan(&raw))
	require.NotContains(t, string(raw), "sk-first")

	require.NoError(t, s.Replace(ctx, pool, id, "sk-second"))
	got, err := s.Get(ctx, pool, id)
	require.NoError(t, err)
	require.Equal(t, "sk-second", got)

	require.NoError(t, s.Delete(ctx, pool, id))
	_, err = s.Get(ctx, pool, id)
	require.ErrorIs(t, err, ErrSecretNotFound)
}

func TestStore_GetMissingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)

	_, err = store.Get(context.Background(), pool, uuid.New())
	require.ErrorIs(t, err, ErrSecretNotFound)
}

func TestStore_ReplaceMissingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := context.Background()
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)
	id := uuid.New()

	err = store.Replace(ctx, pool, id, "replacement")
	var exists bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM secrets WHERE id = $1)", id).Scan(&exists))
	require.False(t, exists)
	require.ErrorIs(t, err, ErrSecretNotFound)
}

func TestStore_DeleteMissingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)

	require.NoError(t, store.Delete(context.Background(), pool, uuid.New()))
}

func TestSealUsesFreshNonce(t *testing.T) {
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	first, err := c.Seal([]byte("secret"), []byte("row"))
	require.NoError(t, err)
	second, err := c.Seal([]byte("secret"), []byte("row"))
	require.NoError(t, err)
	require.NotEqual(t, first.Nonce, second.Nonce)
	require.NotEqual(t, first.Ciphertext, second.Ciphertext)
}
func TestOpenRejectsTamperedOrMalformedSealed(t *testing.T) {
	for _, alteration := range []string{"nonce", "ciphertext", "empty nonce", "short nonce", "long nonce"} {
		t.Run(alteration, func(t *testing.T) {
			c, err := NewCipher("v1", randomKey(t))
			require.NoError(t, err)
			sealed, err := c.Seal([]byte("secret"), []byte("row"))
			require.NoError(t, err)
			switch alteration {
			case "nonce":
				sealed.Nonce[0] ^= 1
			case "ciphertext":
				sealed.Ciphertext[0] ^= 1
			case "empty nonce":
				sealed.Nonce = nil
			case "short nonce":
				sealed.Nonce = sealed.Nonce[:len(sealed.Nonce)-1]
			case "long nonce":
				sealed.Nonce = append(sealed.Nonce, 0)
			}
			_, err = c.Open(sealed, []byte("row"))
			require.ErrorIs(t, err, ErrDecrypt)
		})
	}
}
func TestStoreRejectsCiphertextCopiedToAnotherRow(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := context.Background()
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)
	source, err := store.Put(ctx, pool, "source-secret")
	require.NoError(t, err)
	target, err := store.Put(ctx, pool, "target-secret")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE secrets SET key_id = s.key_id, nonce = s.nonce, ciphertext = s.ciphertext FROM secrets s WHERE secrets.id = $1 AND s.id = $2`, target, source)
	require.NoError(t, err)
	_, err = store.Get(ctx, pool, target)
	require.ErrorIs(t, err, ErrDecrypt)
	got, err := store.Get(ctx, pool, source)
	require.NoError(t, err)
	require.Equal(t, "source-secret", got)
}
func TestStoreSupportsTransactionRollback(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := context.Background()
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)
	existing, err := store.Put(ctx, pool, "original")
	require.NoError(t, err)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	created, err := store.Put(ctx, tx, "created")
	require.NoError(t, err)
	require.NoError(t, store.Replace(ctx, tx, existing, "changed"))
	got, err := store.Get(ctx, tx, existing)
	require.NoError(t, err)
	require.Equal(t, "changed", got)
	require.NoError(t, store.Delete(ctx, tx, existing))
	require.NoError(t, tx.Rollback(ctx))
	got, err = store.Get(ctx, pool, existing)
	require.NoError(t, err)
	require.Equal(t, "original", got)
	_, err = store.Get(ctx, pool, created)
	require.Error(t, err)
}
