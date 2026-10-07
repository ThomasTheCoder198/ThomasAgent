# Auth DTO

## Purpose
Defines the HTTP request and response shapes for authentication.

## Entry points
`LoginRequest`, `UserResponse`, `LoginResponse`, `FromUser`, `FromIssuedSession`.

## Dependencies
Depends on auth domain models; the auth HTTP handler consumes these DTOs.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
JSON field names stay at this boundary. Passwords are accepted only in request DTOs and are never returned.

## Common failures
Changing JSON field names breaks the shared API contract and handler tests.
