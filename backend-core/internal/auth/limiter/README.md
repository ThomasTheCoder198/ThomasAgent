# Auth limiter

## Purpose
Enforces Redis-backed login attempt limits for email, email plus IP and IP alone.

## Entry points
`NewLoginLimiter`, `NewLimiter`, `Check`, `ResetLogin`, `Identity`.

## Dependencies
Uses Redis, typed auth config, `model.NormalizeEmail`, token length, and named errors.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
Email identities are keyed hashes. Raw email and passwords never enter Redis keys or logs.

## Common failures
Redis failure rejects login rather than silently disabling the limit.
