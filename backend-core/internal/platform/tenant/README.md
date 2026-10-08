# Tenant context

## Purpose
Owns trusted tenant context shared by backend modules. It does not accept client
or model parameters and does not choose a tenant when context is absent.

## Entry points
- `WithID` sets the tenant at a trusted server boundary.
- `ID` returns the tenant or `errors.ErrMissingTenant`; an HTTP boundary normalizes that internal cause to `INTERNAL_ERROR`.
- `PlatformID` identifies the platform tenant for explicit boundary setup.

## Dependencies
- Uses: context and the shared error catalog.
- Used by: auth, audit, vault, llmprovider, startup bootstrap and internal routes.

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth ./internal/platform/vault ./internal/platform/audit -count=1 -p 2
```

## Conventions
Business repositories must read `ID` before querying and return its error.
Startup, unauthenticated platform login, and service-token routes explicitly set
the platform scope. Session middleware derives scope from the trusted session
row found by its token hash; request JSON never selects the tenant.

## Common failures
- Missing context returns `INTERNAL_ERROR`: configure the trusted boundary.
- A cross-tenant row is absent: use the authenticated session scope; never fall
back to platform data to bypass isolation.
