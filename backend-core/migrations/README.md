# Core migrations

## Purpose
Owns the versioned Postgres schema embedded in the core executable.
The platform migration establishes the outbox and audit event tables.
The identity/registry migration establishes users, sessions, secrets, providers,
models, and tenant-specific model roles.

## Entry points
- `FS` embeds all SQL files for the Postgres migration provider.
- `20261003000001_platform.sql` creates `outbox` and `audit_events`.
- `20261003000002_identity_registry.sql` creates the six identity/registry tables and their constraints.
- `task migrate` applies pending migrations through Compose.
- `core migrate status` reports applied and pending migration versions.

## Dependencies
- Uses: Go embed and Postgres SQL, interpreted by Goose.
- Used by: `cmd/core` and platform Postgres integration tests.

## Run & test
Add a file YYYYMMDDHHMMSS_<name>.sql with -- +goose Up and -- +goose Down, then run task migrate. Each migration must run up and down in dev.

```bash
# From the repository root:
task migrate
# Outside Docker, export the required CORE_* process variables first:
cd backend-core
go run ./cmd/core migrate status
go run ./cmd/core migrate down
go run ./cmd/core migrate up
go test ./internal/platform/postgres/... -count=1
```
The migrate command defaults to up. Down rolls back the latest migration.
A migration error causes the command to exit unsuccessfully.

## Conventions
Name files in monotonically increasing timestamp order.
Never edit an applied migration: add a new migration instead.
Breaking schema changes follow expand/contract: add columns, write both,
backfill, switch readers, then remove obsolete columns in a later migration.
Keep development seeds separate from schema migrations.

## Common failures
- SQL failure: the migration returns an error; inspect the unapplied SQL.
- Missing database configuration: set the required core environment variables.
- Down removes the latest migration's tables and their data; two successive downs remove identity/registry, then platform tables. Use only on disposable dev data.
