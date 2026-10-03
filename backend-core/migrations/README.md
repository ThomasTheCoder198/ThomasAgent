# Core migrations

## Purpose
Owns the versioned Postgres schema embedded in the core executable.
The platform migration establishes the outbox and audit event tables.
The complete desired application schema lives in `../schema.sql`, outside the
embedded migration directory. Edit that file and generate a new diff migration.

## Entry points
- `FS` embeds all SQL files for the Postgres migration provider.
- `20261003000001_platform.sql` creates `outbox` and `audit_events`.
- `task migration:diff NAME=add_feature` generates timestamped Up/Down migrations
  after comparing replayed history with schema.sql on disposable databases.

## Dependencies
- Uses: Go embed and Postgres SQL, interpreted by goose.
- Used by: `cmd/core` and platform Postgres integration tests.

## Run & test
```bash
cd backend-core
# Set CORE_DATABASE_URL and CORE_REDIS_URL in the environment.
go run ./cmd/core migrate up
go run ./cmd/core migrate status
go run ./cmd/core migrate down
go test ./internal/platform/postgres/... -count=1
# From the repository root, after editing backend-core/schema.sql:
task migration:diff NAME=add_feature
```
The migrate command defaults to up. Down rolls back the latest migration.
A migration error causes the command to exit unsuccessfully.

## Conventions
Name files `YYYYMMDDHHMMSS_<desc>.sql` in monotonically increasing order.
Include `-- +goose Up` and `-- +goose Down` sections.
Never edit an applied migration: add a new migration instead.
Breaking schema changes follow expand/contract: add columns, write both,
backfill, switch readers, then remove obsolete columns in a later migration.
Keep development seeds separate from schema migrations.
Generation never alters the application database. It validates Up matches the
desired schema and Down restores the previous schema before creating output.
See `../internal/platform/schemadiff/README.md` for supported objects and limits.

## Common failures
- SQL failure: the migration returns an error; inspect the unapplied SQL.
- Missing database configuration: set the required core environment variables.
- Down removes platform tables and their data; use only on disposable dev data.
