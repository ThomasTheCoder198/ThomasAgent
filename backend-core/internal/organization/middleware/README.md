# Organization middleware

## Purpose
`RequireOrg`: the only place a path org becomes tenant context.

## Entry points
- `RequireOrg(lookup)`, `OrganizationLookup`, `OrganizationFromContext`, `PathParamOrgID` (`org_middleware.go`)

## Dependencies
- Uses: `organization/model`, `platform/httpx`, `platform/tenant`
- Used by: `cmd/core` routes under `/api/v1/orgs/{orgId}/apps/...` (the app proxy), behind `RequireSession`

## Run & test
```bash
cd backend-core
go test ./internal/organization/middleware -count=1
```

## Conventions
It looks the org up under the session's platform tenant (`default`, an unknown or a malformed id is `NOT_FOUND`), refuses an org ragx has not provisioned with `ORG_NOT_PROVISIONED` (no ragx call), then replaces the tenant in context with the org's id.
