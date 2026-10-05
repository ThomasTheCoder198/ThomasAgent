# Core migrations

## Purpose
Owns the versioned Postgres schema embedded in the core executable.
The platform migration establishes the outbox and audit event tables.
The identity/registry migration establishes users, sessions, secrets, providers,
models, and tenant-specific model roles.
Sessions reference users with the composite `(tenant_id, user_id)` foreign key, and session/audit/registry repository queries explicitly scope the Platform tenant.

`20261004000002_tenant_context_required.sql` removes all eight business-table
`tenant_id` defaults. Every INSERT supplies trusted context scope; Down restores
the old defaults without modifying row ownership.

## Entry points
- `FS` embeds all SQL files for the Postgres migration provider.
- `20261003000001_platform.sql` creates `outbox` and `audit_events`.
- `20261003000002_identity_registry.sql` creates the six identity/registry tables and their constraints.
- `20261005000001_vault_aad_version.sql` expands the AAD version constraint to permit version 3; Down intentionally retains that expansion and all encrypted data.
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
- Down removes one migration at a time. From the latest schema, AAD expansion, tenant defaults and hardening constraints roll back first; the fourth and fifth Down remove identity/registry and platform tables and their data. Use only on disposable dev data.

`20261004000001_identity_hardening.sql` upgrades already deployed identity tables with the composite session/user tenant foreign key and a vault AAD format marker. Original migrations remain unchanged. Down restores the old FK while retaining `aad_version` to prevent ciphertext misclassification; Up is safe after Down. After a secret is resealed, old application binaries require an encrypted-data backup to roll back.

**AAD downgrade is irreversible by SQL alone.** Migration Down does not reseal
version 2 ciphertext: database migrations cannot access the vault master key.
An old application binary that uses UUID-only AAD cannot decrypt those rows.
Before rolling back the binary, restore a pre-upgrade encrypted-data backup or
run an authenticated reseal procedure with the master key. Keep the current
binary when exercising schema Down/Up; the format marker remains intact.
`TestHardeningUpDownUpPreservesSeededBoundCiphertext` verifies seeded version 2
data remains decryptable after schema Up/Down/Up, and migration-status tests
verify the new tenant migration is applied and rolled back independently.

Version 3 uses length-prefixed AAD; the new migration keeps the version 3 check on Down because narrowing it would invalidate existing rows. An older binary that only reads version 2 also needs a backup or authenticated reseal before downgrade. `TestStructuredAADMigrationUpDownUpPreservesVersionThree` checks expansion, marker/data preservation and rejection of unknown versions across Up/Down/Up.
