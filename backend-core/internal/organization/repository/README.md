# Organization repository

## Purpose
SQL for the `organizations` table. Every method takes the pool or the service transaction and scopes rows by the tenant in ctx, which is `default` for the operator.

## Entry points
- `OrganizationRepo` (`organization_repository.go`)
- `MarkSynced` compares the version ragx holds with the current row; `MarkSyncFailed` applies only to the version sent. `UpdateSettings` increments `settings_version` once per real change.

## Dependencies
- Uses: `organization/model`, `platform/postgres`, `platform/tenant`
- Used by: `organization/service`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/organization/repository -count=1
```

## Conventions
A duplicate name (case-insensitive) becomes `CONFLICT` with the field `name` / `TAKEN`. Status values are bound from the `model.RagxStatus*` constants; only the migration's CHECK keeps literals.
