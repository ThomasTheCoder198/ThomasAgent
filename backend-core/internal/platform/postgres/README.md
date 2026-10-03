# Postgres

## Purpose
Owns creation of a verified Postgres connection pool and execution of embedded
SQL migrations. Business queries remain in their consuming modules.

## Entry points
- `Open(ctx, url)` creates a pool and pings before returning it.
- `Migrate(ctx, url, fsys, direction)` runs goose up, down, or status.
- `Up`, `Down`, `Status` define supported migration directions.

## Dependencies
- Uses: pgx v5, goose v3, and Postgres.
- Used by: `cmd/core` for startup, readiness, and the migrate command.
- Tests use testcontainers and the embedded `migrations.FS`.

## Run & test
```bash
cd backend-core
go test ./internal/platform/postgres/... -count=1
```
Docker Desktop must run with Linux containers; tests use postgres:18.6-alpine.
Tests apply and roll back the platform migration, reject deliberately broken
SQL, and check that connection failure is returned before a pool is exposed.

## Conventions
The core command wraps migration and startup connection/ping operations in
client spans and logs failures once at that boundary. This package returns errors.
The caller closes a returned pool. Failed ping closes it internally.
Migration files use `YYYYMMDDHHMMSS_<desc>.sql` and goose Up/Down annotations.
`Up` applies all pending migrations; `Down` rolls back one migration.
`Status` validates/query-checks migration status; this API discards the rows.
Never edit an applied migration. Use expand/contract for breaking changes.

## Common failures
- Pool startup fails: verify `CORE_DATABASE_URL` and Postgres availability.
- Migration fails: fix an unapplied migration or add a corrective migration.
- Integration tests cannot start: check Docker Desktop and image access.
