# Schema migration generation

## Purpose
Computes a real PostgreSQL schema diff from existing Goose migration history to
`backend-core/schema.sql`, generating a reversible Goose migration after checking
both directions. Uses disposable PostgreSQL databases and never connects to the
application database.

## Entry points
- `New` injects typed settings, a UTC clock source and a tracer.
- `Generator.Generate` validates inputs, replays history, generates and validates
  Up/Down, and creates the migration exclusively. An empty Path means no changes.

## Dependencies
- Uses: Atlas `ariga.io/atlas v1.3.0`, Goose, pgx and testcontainers PostgreSQL.
- Errors use the canonical `internal/errors` named definitions; validation failures
  carry `ErrValidationFailed` and infrastructure failures normalize to `ErrInternalError`.
- Used by: `cmd/migrationgen` and `task migration:diff`.

## Run & test
```bash
task migration:diff NAME=add_feature
cd backend-core
go test ./internal/platform/schemadiff ./cmd/migrationgen -count=1
```
Docker Desktop must be running. `CORE_SCHEMA_POSTGRES_IMAGE` defaults to the same
PostgreSQL image as development; generation and cleanup have separate deadlines.

## Conventions
Edit schema.sql for the desired complete application schema, then generate a new
`YYYYMMDDHHMMSS_snake_case.sql` file. Generation checks UTC timestamps exceed every
existing migration version and refuses to overwrite files. Goose metadata and
its owned sequence are excluded from the history comparison.

The supported automatic diff covers public-schema tables, columns, indexes,
constraints, enum types and comments. Atlas Community has limits for advanced
objects; raw PostgreSQL catalog checks reject views, materialized views,
all application sequences (including serial/identity-owned sequences), partitions,
foreign tables, functions, procedures,
triggers, extensions, RLS, domains, custom base/range/composite types, collations,
operators, custom operator families/classes, explicit grants, default privileges,
additional schemas and virtual generated columns. These need a reviewed manual Goose
migration. Unsupported objects are never silently accepted as an empty diff.

Generated Down restores schema structure, not data deleted by Up. Review generated
SQL before deployment. Application of migrations remains `task migrate`.

## Common failures
- Docker unavailable: start Docker Desktop; generation does not access dev data.
- Timestamp collision or future migration: wait for the next UTC second or fix the clock.
- Invalid SQL: correct schema.sql; no migration is created.
- Unsupported or nontransactional plan: write a reviewed manual migration.
- Up/Down mismatch: generation refuses output; fix the schema or migration plan.
