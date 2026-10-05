package vault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
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
	c3, err := NewCipher("v2", randomKey(t))
	require.NoError(t, err)
	_, err = c3.Open(sealed, []byte("row-1"))
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestOpenRejectsMismatchedKeyIDWithSameKeyBytes(t *testing.T) {
	key := randomKey(t)
	first, err := NewCipher("v1", key)
	require.NoError(t, err)
	second, err := NewCipher("v2", key)
	require.NoError(t, err)
	sealed, err := first.Seal([]byte("secret"), []byte("row"))
	require.NoError(t, err)
	_, err = second.Open(sealed, []byte("row"))
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStorePersistsEncrypted(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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

func TestStoreUpgradesLegacyAADWithoutLosingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	cipher, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	id := uuid.New()
	legacy, err := cipher.Seal([]byte("existing-provider-key"), id[:])
	require.NoError(t, err)
	_, err = pool.Exec(tenant.WithID(t.Context(), tenant.PlatformID), `INSERT INTO secrets (tenant_id,id,key_id,nonce,ciphertext) VALUES ('default',$1,$2,$3,$4)`, id, legacy.KeyID, legacy.Nonce, legacy.Ciphertext)
	require.NoError(t, err)
	value, err := NewStore(cipher).Get(tenant.WithID(t.Context(), tenant.PlatformID), pool, id)
	require.NoError(t, err)
	require.Equal(t, "existing-provider-key", value)
	var upgraded Sealed
	require.NoError(t, pool.QueryRow(tenant.WithID(t.Context(), tenant.PlatformID), `SELECT key_id,nonce,ciphertext FROM secrets WHERE id=$1`, id).Scan(&upgraded.KeyID, &upgraded.Nonce, &upgraded.Ciphertext))
	// Independently encode the v3 wire format, rather than call secretAAD.
	var aad []byte
	for _, field := range []string{"default", id.String(), "v1"} {
		var prefix [binary.MaxVarintLen64]byte
		length := binary.PutUvarint(prefix[:], uint64(len(field)))
		aad = append(aad, prefix[:length]...)
		aad = append(aad, field...)
	}
	plain, err := cipher.Open(upgraded, aad)
	require.NoError(t, err)
	require.Equal(t, "existing-provider-key", string(plain))
	_, err = cipher.Open(upgraded, []byte("default:"+id.String()+":v1"))
	require.ErrorIs(t, err, ErrDecrypt)
	_, err = cipher.Open(upgraded, id[:])
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStoreScopesAndAuthenticatesTenant(t *testing.T) {
	pool := pgtest.Start(t)
	cipher, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(cipher)
	otherCtx := tenant.WithID(t.Context(), "other")
	id, err := store.Put(otherCtx, pool, "other-secret")
	require.NoError(t, err)
	_, err = store.Get(tenant.WithID(t.Context(), tenant.PlatformID), pool, id)
	require.ErrorIs(t, err, ErrSecretNotFound)
	require.ErrorIs(t, store.Replace(tenant.WithID(t.Context(), tenant.PlatformID), pool, id, "wrong"), ErrSecretNotFound)
	require.ErrorIs(t, store.Delete(tenant.WithID(t.Context(), tenant.PlatformID), pool, id), ErrSecretNotFound)
	value, err := store.Get(otherCtx, pool, id)
	require.NoError(t, err)
	require.Equal(t, "other-secret", value)
	_, err = pool.Exec(tenant.WithID(t.Context(), tenant.PlatformID), `UPDATE secrets SET tenant_id='default' WHERE id=$1`, id)
	require.NoError(t, err)
	_, err = store.Get(tenant.WithID(t.Context(), tenant.PlatformID), pool, id)
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStoreRejectsMissingTenant(t *testing.T) {
	pool := pgtest.Start(t)
	cipher, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(cipher)
	_, err = store.Put(t.Context(), pool, "secret")
	require.ErrorIs(t, err, tenant.ErrMissingTenant, "missing tenant must not insert in the platform scope")
	_, err = store.Get(t.Context(), pool, uuid.New())
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
	require.ErrorIs(t, store.Replace(t.Context(), pool, uuid.New(), "secret"), tenant.ErrMissingTenant)
	require.ErrorIs(t, store.Delete(t.Context(), pool, uuid.New()), tenant.ErrMissingTenant)
}

func TestStoreDeleteCannotCrossTenants(t *testing.T) {
	pool := pgtest.Start(t)
	cipher, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(cipher)
	otherCtx := tenant.WithID(t.Context(), "other")
	id, err := store.Put(otherCtx, pool, "other-secret")
	require.NoError(t, err)
	require.ErrorIs(t, store.Delete(tenant.WithID(t.Context(), tenant.PlatformID), pool, id), ErrSecretNotFound)
	value, err := store.Get(otherCtx, pool, id)
	require.NoError(t, err)
	require.Equal(t, "other-secret", value)
}

func TestStore_GetMissingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	c, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	store := NewStore(c)

	_, err = store.Get(tenant.WithID(context.Background(), tenant.PlatformID), pool, uuid.New())
	require.ErrorIs(t, err, ErrSecretNotFound)
}

func TestStore_ReplaceMissingSecret(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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

	require.ErrorIs(t, store.Delete(tenant.WithID(context.Background(), tenant.PlatformID), pool, uuid.New()), ErrSecretNotFound)
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
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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

func TestSecretAAD_ColonFieldsCannotCollide(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	a := secretAAD("org:"+id.String()+":team", id, "v1")
	b := secretAAD("org", id, "team:"+id.String()+":v1")
	require.NotEqual(t, a, b, "tenant and key boundaries must be authenticated unambiguously")
	cipher, err := NewCipher("v1", randomKey(t))
	require.NoError(t, err)
	sealed, err := cipher.Seal([]byte("secret"), a)
	require.NoError(t, err)
	_, err = cipher.Open(sealed, b)
	require.ErrorIs(t, err, ErrDecrypt)
}

func TestStore_UpgradesPreviousBoundAAD(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(t.Context(), "org:team")
	cipher, err := NewCipher("key:v1", randomKey(t))
	require.NoError(t, err)
	id := uuid.New()
	old, err := cipher.Seal([]byte("old-bound-secret"), []byte("org:team:"+id.String()+":key:v1"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO secrets (tenant_id,id,key_id,nonce,ciphertext,aad_version) VALUES ($1,$2,$3,$4,$5,2)`, "org:team", id, old.KeyID, old.Nonce, old.Ciphertext)
	require.NoError(t, err)
	store := NewStore(cipher)
	value, err := store.Get(ctx, pool, id)
	require.NoError(t, err)
	require.Equal(t, "old-bound-secret", value)
	var version int
	var upgraded Sealed
	require.NoError(t, pool.QueryRow(ctx, `SELECT aad_version,key_id,nonce,ciphertext FROM secrets WHERE id=$1`, id).Scan(&version, &upgraded.KeyID, &upgraded.Nonce, &upgraded.Ciphertext))
	require.Equal(t, 3, version)
	require.NotEqual(t, old.Nonce, upgraded.Nonce)
	require.NotEqual(t, old.Ciphertext, upgraded.Ciphertext)
	_, err = cipher.Open(upgraded, []byte("org:team:"+id.String()+":key:v1"))
	require.ErrorIs(t, err, ErrDecrypt)
	value, err = store.Get(ctx, pool, id)
	require.NoError(t, err)
	require.Equal(t, "old-bound-secret", value)
	var secondNonce []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT nonce FROM secrets WHERE id=$1`, id).Scan(&secondNonce))
	require.Equal(t, upgraded.Nonce, secondNonce, "current envelopes must not be resealed on every read")
}
