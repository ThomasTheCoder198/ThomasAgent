# Auth repository

## Purpose
Owns tenant-scoped SQL for users and sessions.

## Entry points
`AuthRepository` interface and `NewAuthRepository` constructor.

## Dependencies
Uses Postgres DBTX, auth models and named catalog errors. The auth service consumes the repository.

## Run & test
Covered by `go test ./internal/auth/...` with migrated Postgres testcontainers.

## Conventions
Methods derive tenant identity from context. `FindSessionByToken` first identifies a session's trusted tenant; `PurgeAllExpiredSessions` is reserved for the system job.

## Common failures
Missing tenant context fails closed for ordinary reads and writes.
