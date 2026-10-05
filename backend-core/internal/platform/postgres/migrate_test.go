package postgres

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const postgresImage = "postgres:18.6-alpine"

func TestMigrationUpgradesExistingIdentitySchema(t *testing.T) {
	url := startPostgres(t)
	legacy := fstest.MapFS{}
	for _, name := range []string{"20261003000001_platform.sql", "20261003000002_identity_registry.sql"} {
		raw, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		legacy[name] = &fstest.MapFile{Data: raw}
	}
	require.NoError(t, Migrate(t.Context(), url, legacy, Up))
	pool, err := Open(t.Context(), url)
	require.NoError(t, err)
	defer pool.Close()
	var userID string
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO users (email,password_hash) VALUES ('old@example.com','hash') RETURNING id`).Scan(&userID))
	require.NoError(t, Migrate(t.Context(), url, migrations.FS, Up))
	_, err = pool.Exec(t.Context(), `INSERT INTO sessions (tenant_id,user_id,token_hash,csrf_token,expires_at) VALUES ('other',$1,'token','csrf',now()+interval '1 hour')`, userID)
	require.Error(t, err, "an upgrade must reject a session referencing another tenant's user")
}

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("thomas"), tcpostgres.WithUsername("thomas"), tcpostgres.WithPassword("thomas"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func TestMigrate_UpAndDown(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))

	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&n))

	// Down rolls back AAD expansion, tenant defaults, hardening, identity/registry, then platform.
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&n), "platform tables survive the first Down")
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&n)
	require.Error(t, err)
}

func TestMigrate_UpFailsOnBrokenMigration(t *testing.T) {
	url := startPostgres(t)
	broken := fstest.MapFS{
		"20260101000001_ok.sql":     {Data: []byte("-- +goose Up\nCREATE TABLE a (id int);\n-- +goose Down\nDROP TABLE a;\n")},
		"20260101000002_broken.sql": {Data: []byte("-- +goose Up\nCREATE TABLE b (id nonexistent_type);\n-- +goose Down\nDROP TABLE b;\n")},
	}
	err := Migrate(context.Background(), url, broken, Up)
	require.Error(t, err)
}

func TestMigrationStatuses_ReturnsMigrationStates(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	statuses, err := MigrationStatuses(ctx, url, migrations.FS)
	require.NoError(t, err)
	require.Len(t, statuses, 5)
	require.EqualValues(t, 20261003000001, statuses[0].Source.Version)
	require.Equal(t, "20261003000001_platform.sql", statuses[0].Source.Path)
	require.EqualValues(t, 20261003000002, statuses[1].Source.Version)
	require.Equal(t, "20261003000002_identity_registry.sql", statuses[1].Source.Path)
	require.EqualValues(t, 20261004000002, statuses[3].Source.Version)
	require.Equal(t, "20261004000002_tenant_context_required.sql", statuses[3].Source.Path)
	require.EqualValues(t, 20261005000001, statuses[4].Source.Version)
	require.Equal(t, "20261005000001_vault_aad_version.sql", statuses[4].Source.Path)
	for _, status := range statuses {
		require.Equal(t, goose.StatePending, status.State)
	}

	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	statuses, err = MigrationStatuses(ctx, url, migrations.FS)
	require.NoError(t, err)
	require.Len(t, statuses, 5)
	for _, status := range statuses {
		require.Equal(t, goose.StateApplied, status.State)
	}

	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	statuses, err = MigrationStatuses(ctx, url, migrations.FS)
	require.NoError(t, err)
	require.Len(t, statuses, 5)
	require.Equal(t, goose.StateApplied, statuses[0].State)
	require.Equal(t, goose.StateApplied, statuses[1].State)
	require.Equal(t, goose.StateApplied, statuses[2].State)
	require.Equal(t, goose.StateApplied, statuses[3].State)
	require.Equal(t, goose.StatePending, statuses[4].State)
}

func TestTenantMigrationRequiresExplicitTenantAndRestoresDefaultsOnDown(t *testing.T) {
	url := startPostgres(t)
	ctx := t.Context()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	for _, table := range []string{"outbox", "audit_events", "users", "sessions", "secrets", "providers", "models", "model_roles"} {
		var absent bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT column_default IS NULL FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='tenant_id'`, table).Scan(&absent))
		require.True(t, absent, table+" must not silently select platform tenant")
	}
	_, err = pool.Exec(ctx, `INSERT INTO users (email,password_hash) VALUES ('missing@example.com','hash')`)
	require.Error(t, err)
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	_, err = pool.Exec(ctx, `INSERT INTO users (email,password_hash) VALUES ('legacy@example.com','hash')`)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	var scope string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tenant_id FROM users WHERE email='legacy@example.com'`).Scan(&scope))
	require.Equal(t, "default", scope)
}

func TestHardeningUpDownUpPreservesSeededBoundCiphertext(t *testing.T) {
	url := startPostgres(t)
	ctx := t.Context()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	block, err := aes.NewCipher(bytes.Repeat([]byte{1}, 32))
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := bytes.Repeat([]byte{2}, 12)
	aad := []byte("other:11111111-1111-1111-1111-111111111111:v1")
	sealed := gcm.Seal(nil, nonce, []byte("seeded-provider-secret"), aad)
	_, err = pool.Exec(ctx, `INSERT INTO secrets (tenant_id,id,key_id,nonce,ciphertext,aad_version) VALUES ('other','11111111-1111-1111-1111-111111111111','v1',$1,$2,2)`, nonce, sealed)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	var version int
	var storedNonce, storedCiphertext []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT aad_version,nonce,ciphertext FROM secrets WHERE id='11111111-1111-1111-1111-111111111111'`).Scan(&version, &storedNonce, &storedCiphertext))
	require.Equal(t, 2, version, "down must not erase the AAD format marker")
	plain, err := gcm.Open(nil, storedNonce, storedCiphertext, aad)
	require.NoError(t, err)
	require.Equal(t, "seeded-provider-secret", string(plain))
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	require.NoError(t, pool.QueryRow(ctx, `SELECT aad_version,nonce,ciphertext FROM secrets WHERE id='11111111-1111-1111-1111-111111111111'`).Scan(&version, &storedNonce, &storedCiphertext))
	require.Equal(t, 2, version)
	plain, err = gcm.Open(nil, storedNonce, storedCiphertext, aad)
	require.NoError(t, err)
	require.Equal(t, "seeded-provider-secret", string(plain))
}

func TestOpen_FailsFastOnBadURL(t *testing.T) {
	_, err := Open(context.Background(), "postgres://nobody:nothing@127.0.0.1:1/none?connect_timeout=1")
	require.Error(t, err)
}

func TestIdentityRegistryTablesExist(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	for _, table := range []string{"users", "sessions", "secrets", "providers", "models", "model_roles"} {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists))
		require.True(t, exists, table)
	}
}

func TestStructuredAADMigrationUpDownUpPreservesVersionThree(t *testing.T) {
	url := startPostgres(t)
	ctx := t.Context()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `INSERT INTO secrets (tenant_id,id,key_id,nonce,ciphertext,aad_version) VALUES ('default','11111111-1111-1111-1111-111111111111','v1',$1,$2,3)`, []byte("nonce"), []byte("ciphertext"))
	require.NoError(t, err, "new envelopes require an expanded version constraint")
	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	var version int
	var nonce, ciphertext []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT aad_version,nonce,ciphertext FROM secrets WHERE id='11111111-1111-1111-1111-111111111111'`).Scan(&version, &nonce, &ciphertext))
	require.Equal(t, 3, version)
	require.Equal(t, []byte("nonce"), nonce)
	require.Equal(t, []byte("ciphertext"), ciphertext)
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))
	_, err = pool.Exec(ctx, `UPDATE secrets SET aad_version=4 WHERE id='11111111-1111-1111-1111-111111111111'`)
	require.Error(t, err, "unknown AAD versions must remain constrained")
}
