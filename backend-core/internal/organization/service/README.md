# Organization service

## Purpose
Business rules and transactions for organizations, and the synchronous ragx provisioning call.

## Entry points
- `OrganizationService` (`organization_service.go`): list, get, create, update, provision
- `RagxProvisioner`: the one ragx call this service makes (satisfied by `*ragx.Caller`)
- Audit action names (`audit_events.go`)

## Dependencies
- Uses: `organization/repository`, `platform/ragx`, `platform/retry`, `platform/audit`, `platform/postgres`, `platform/tenant`
- Used by: `organization/controller`, `organization/middleware` (as `OrganizationLookup`), `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/organization/service -count=1
```

## Conventions
Never hold a transaction across the ragx call: commit the row, then provision. The call runs under the org's own id as tenant (the App token's `oid`) on a context detached from the request's cancellation. Ids that are not UUIDs, including `default`, are `NOT_FOUND` before any query.
