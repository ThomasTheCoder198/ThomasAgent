# Postgres

## Purpose
Owns creation of a verified Postgres connection pool and execution of embedded
SQL migrations. Business queries remain in their consuming modules.

## Entry points
- `Open(ctx, url)` creates a pool and pings before returning it.
- `Migrate(ctx, url, fsys, direction)` runs goose up or down.
- `Up`, `Down` define supported migration directions.
- `MigrationStatuses(ctx, url, fsys)` returns goose migration versions, source paths, and states for callers to inspect or display.
- `DBTX` supplies the query methods shared by pgx pools and transactions.
- `IsUniqueViolation` and `IsForeignKeyViolation` recognize wrapped Postgres constraint errors, including delete restrictions.
- `pgtest.Start(t)` supplies a disposable, fully migrated pool to consuming modules' tests.

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
Tests apply and roll back both platform and identity/registry migrations, reject deliberately broken
SQL, verify status transitions from pending to applied and back, and check that connection failure is returned before a pool is exposed.

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
The core command wraps migration and startup connection/ping operations in
client spans and logs failures once at that boundary. This package returns errors.
The caller closes a returned pool. Failed ping closes it internally.
Migration files use `YYYYMMDDHHMMSS_<desc>.sql` and goose Up/Down annotations.
`Up` applies all pending migrations; `Down` rolls back one migration.
`MigrationStatuses` returns every migration row instead of discarding status results.
Never edit an applied migration. Use expand/contract for breaking changes.

## Common failures
- Pool startup fails: verify `CORE_DATABASE_URL` and Postgres availability.
- Migration fails: fix an unapplied migration or add a corrective migration.
- Integration tests cannot start: check Docker Desktop and image access.
