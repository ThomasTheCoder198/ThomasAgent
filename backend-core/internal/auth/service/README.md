# Auth service

## Purpose
Owns auth business orchestration and the behaviors required by the HTTP boundary.

## Entry points
`Service`, `NewAuthService`, `SessionAuthenticator` and `AuthService` interfaces.

## Dependencies
Depends on auth models and repository, typed config, Argon2id, audit and named errors.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
The consumer declares only methods it calls. The implementation handles password hashing, sessions and audit transactions.

## Common failures
Login errors must use named catalog errors and never reveal whether an email exists.
