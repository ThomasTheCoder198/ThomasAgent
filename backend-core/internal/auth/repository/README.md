# Auth repository

## Purpose
SQL for users and sessions. Pure data access: no business rules and no transactions of its own.

## Entry points
- `UserRepo`, `NewUserRepo` (`user_repository.go`)
- `SessionRepo`, `NewSessionRepo` (`session_repository.go`)

## Dependencies
- Uses: `auth/model`, `platform/postgres`, `platform/tenant`
- Used by: `auth/service`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth/repository -count=1
```

## Conventions
Every method is scoped to the tenant in ctx, except `FindSessionAcrossTenants` (authentication) and `PurgeExpiredSessionsAcrossTenants` (purge job).
