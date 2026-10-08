# Auth service

## Purpose
Business rules of login: owner bootstrap, password checks, session issue/authenticate/logout and purge. Owns transactions.

## Entry points
- `AuthService`, `NewAuthService` (`auth_service.go`)
- `passwordHasher`: Argon2id under a concurrency budget with a timing decoy (`password_hasher.go`)
- `newRandomToken` (`session_token.go`)

## Dependencies
- Uses: `auth/repository`, `auth/model`, `platform/audit`, `platform/config`
- Used by: `auth/controller`, `auth/middleware`, `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth/service -count=1
```

## Conventions
Session creation and its audit row share one transaction; a failed rollback keeps both causes.
