package schemadiff

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func testGenerator(t *testing.T) *Generator {
	t.Helper()
	cfg, err := config.LoadSchema()
	require.NoError(t, err)
	return New(cfg, func() time.Time { return time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC) }, sdktrace.NewTracerProvider().Tracer("schema-test"))
}

func TestMigrationNameAndVersionValidation(t *testing.T) {
	for _, name := range []string{"", "AddTable", "../escape", "add-table", "_table", "table__name", "table_"} {
		t.Run(name, func(t *testing.T) {
			_, err := migrationFilename(name, time.Now(), nil)
			require.Error(t, err)
		})
	}
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.FixedZone("test", 3600))
	name, err := migrationFilename("add_table", now, []string{"20261003000001_platform.sql"})
	require.NoError(t, err)
	require.Equal(t, "20261003090000_add_table.sql", name)
	_, err = migrationFilename("add_table", now, []string{"20261003090000_existing.sql"})
	require.ErrorContains(t, err, "later than")
}

func TestWriteMigrationNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "20261003100000_add_table.sql")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
	err := writeMigration(path, []byte("replacement"))
	require.Error(t, err)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(contents))
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestPublishMigrationWriteFailureLeavesNoSQLOrTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20261003100000_add_table.sql")
	err := publishMigration(path, func(file *os.File) error {
		_, writeErr := file.WriteString("partial SQL")
		require.NoError(t, writeErr)
		return io.ErrShortWrite
	})
	require.ErrorIs(t, err, io.ErrShortWrite)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestGenerateInfrastructureFailureUsesInternalCode(t *testing.T) {
	dir := t.TempDir()
	migrationDir := filepath.Join(dir, "migrations")
	require.NoError(t, os.Mkdir(migrationDir, 0700))
	schemaPath := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaPath, []byte("CREATE TABLE items (id integer);"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testGenerator(t).Generate(ctx, Request{MigrationName: "add_items", SchemaPath: schemaPath, MigrationsDir: migrationDir})
	require.Error(t, err)
	require.Equal(t, apperr.CodeInternalError, apperr.From(err).Code)
}

func TestGenerateSchemaMigration(t *testing.T) {
	for _, test := range []struct {
		name      string
		schema    string
		wantSQL   string
		wantError string
	}{
		{name: "zero_diff", schema: "CREATE TABLE items (id integer PRIMARY KEY);"},
		{name: "add_column_with_validated_rollback", schema: "CREATE TABLE items (id integer PRIMARY KEY, title text);", wantSQL: "ADD COLUMN"},
		{name: "invalid_schema", schema: "CREATE TABLE items (id nonexistent_type);", wantError: "desired schema"},
		{name: "unsupported_view", schema: "CREATE TABLE items (id integer PRIMARY KEY); CREATE VIEW item_ids AS SELECT id FROM items;", wantError: "unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			migrationDir := filepath.Join(dir, "migrations")
			require.NoError(t, os.Mkdir(migrationDir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(migrationDir, "20261003000001_items.sql"), []byte("-- +goose Up\nCREATE TABLE items (id integer PRIMARY KEY);\n-- +goose Down\nDROP TABLE items;\n"), 0600))
			schemaPath := filepath.Join(dir, "schema.sql")
			require.NoError(t, os.WriteFile(schemaPath, []byte(test.schema), 0600))
			result, err := testGenerator(t).Generate(context.Background(), Request{MigrationName: "add_title", SchemaPath: schemaPath, MigrationsDir: migrationDir})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Empty(t, result.Path)
				return
			}
			require.NoError(t, err)
			if test.wantSQL == "" {
				require.Empty(t, result.Path)
				return
			}
			require.Equal(t, filepath.Join(migrationDir, "20261003100000_add_title.sql"), result.Path)
			contents, err := os.ReadFile(result.Path)
			require.NoError(t, err)
			require.Contains(t, string(contents), test.wantSQL)
			require.Contains(t, string(contents), "-- +goose Down")
			require.Contains(t, string(contents), "DROP COLUMN")
		})
	}
}
