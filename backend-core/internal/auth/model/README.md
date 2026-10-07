# Auth model

## Purpose
Owns auth domain entities: users, sessions and issued session credentials.

## Entry points
`User`, `Session`, `IssuedSession`, `NormalizeEmail`.

## Dependencies
Go time and UUID only. Used by auth repository, service and DTO mapping.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
Models represent domain state and do not encode HTTP requests or responses.

## Common failures
An expired session must be rejected by the service after repository lookup.
