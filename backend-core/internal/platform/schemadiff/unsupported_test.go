package schemadiff

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func TestRejectUnsupportedCatalogObjects(t *testing.T) {
	settings, err := config.LoadSchema()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), settings.GenerationTimeout)
	defer cancel()
	scratch, err := startScratch(ctx, settings, sdktrace.NewTracerProvider().Tracer("catalog-test"))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), settings.CleanupTimeout)
		defer cleanupCancel()
		require.NoError(t, scratch.close(cleanupCtx))
	})
	for _, test := range []struct{ name, ddl string }{
		{"range_type", "CREATE TYPE score_range AS RANGE (subtype=integer);"},
		{"custom_collation", "CREATE COLLATION custom_sort FROM \"C\";"},
		{"custom_operator", "CREATE OPERATOR public.=== (FUNCTION=int4eq, LEFTARG=integer, RIGHTARG=integer);"},
		{"table_grants", "CREATE TABLE items (id integer); GRANT SELECT ON items TO PUBLIC;"},
		{"default_privileges", "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO PUBLIC;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := scratch.desired.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
			require.NoError(t, err)
			_, err = scratch.desired.ExecContext(ctx, test.ddl)
			require.NoError(t, err)
			require.ErrorContains(t, rejectUnsupportedObjects(ctx, scratch.desired, ""), "unsupported schema object")
		})
	}
}
