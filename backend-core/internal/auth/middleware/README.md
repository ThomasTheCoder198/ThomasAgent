# Auth middleware

## Purpose
Gin middleware that guards routes: browser sessions with CSRF, and internal callers by service token.

## Entry points
- `RequireSession`, `UserFromContext` (`session_middleware.go`)
- `RequireServiceToken` (`service_token_middleware.go`)

## Dependencies
- Uses: `auth/service`, `auth/util`, `platform/httpx`, `platform/tenant`
- Used by: `cmd/core/routes.go`, `auth/controller`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth/middleware -count=1
```

## Conventions
A stale session cookie gets both cookies cleared in the 401. The service token sets the platform tenant.
