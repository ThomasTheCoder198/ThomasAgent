package schemadiff

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// Catalog inspection prevents unsupported objects from disappearing silently from Atlas's inspected schema.
const unsupportedObjectsQuery = `
SELECT kind FROM (
 SELECT 'view, materialized view, sequence, partition or foreign table' AS kind
 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND (c.relkind IN ('v', 'm', 'S', 'p', 'f') OR c.relispartition)
 AND NOT EXISTS (SELECT 1 FROM pg_depend d JOIN pg_class metadata ON metadata.oid = d.refobjid
 JOIN pg_namespace mn ON mn.oid = metadata.relnamespace
 WHERE d.objid = c.oid AND metadata.relname = $1 AND mn.nspname = 'public')
 UNION ALL
 SELECT 'function or procedure' FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
 WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'trigger' FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
 JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND NOT t.tgisinternal
 UNION ALL
 SELECT 'row-level security policy' FROM pg_policy p JOIN pg_class c ON c.oid = p.polrelid
 JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'row-level security' FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND (c.relrowsecurity OR c.relforcerowsecurity)
 UNION ALL
 SELECT 'extension' FROM pg_extension WHERE extname <> 'plpgsql'
 UNION ALL
 SELECT 'domain, range, base or standalone composite type' FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
 WHERE n.nspname = 'public' AND ((t.typtype NOT IN ('e', 'c') AND t.typelem = 0) OR (t.typtype = 'c' AND EXISTS (
 SELECT 1 FROM pg_class c WHERE c.oid = t.typrelid AND c.relkind = 'c')))
 UNION ALL
 SELECT 'custom collation' FROM pg_collation c JOIN pg_namespace n ON n.oid = c.collnamespace
 WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'custom operator' FROM pg_operator o JOIN pg_namespace n ON n.oid = o.oprnamespace
 WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'custom operator class' FROM pg_opclass o JOIN pg_namespace n ON n.oid = o.opcnamespace
 WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'custom operator family' FROM pg_opfamily o JOIN pg_namespace n ON n.oid = o.opfnamespace
 WHERE n.nspname = 'public'
 UNION ALL
 SELECT 'explicit relation privileges' FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND c.relacl IS NOT NULL
 UNION ALL
 SELECT 'default privileges' FROM pg_default_acl
 UNION ALL
 SELECT 'additional application schema' FROM pg_namespace
 WHERE nspname <> 'public' AND nspname <> 'information_schema' AND nspname NOT LIKE 'pg_%'
 UNION ALL
 SELECT 'virtual generated column' FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
 JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND a.attgenerated = 'v'
) unsupported LIMIT 1`

func rejectUnsupportedObjects(ctx context.Context, database *sql.DB, metadataTable string) error {
	var kind string
	err := database.QueryRowContext(ctx, unsupportedObjectsQuery, metadataTable).Scan(&kind)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect unsupported schema objects: %w", err)
	}
	return errors.ErrValidationFailed.WithCause(fmt.Errorf("unsupported schema object: %s; use a reviewed manual Goose migration", kind))
}
