# Auth token

## Purpose
Owns session-token length and the hash used for storage and lookup.

## Entry points
`Bytes` and `Hash`.

## Dependencies
Go standard library only. Used by auth service and limiter.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
Store token hashes; raw tokens remain request-local.

## Common failures
Changing the hash scheme invalidates existing sessions and needs a migration plan.
