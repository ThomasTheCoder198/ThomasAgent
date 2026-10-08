# LLM provider repository

## Purpose
Owns tenant-scoped SQL access for providers, models and role assignments.

## Entry points
`NewLLMProviderRepository` returns `PGRepository`, which implements the focused provider, model and role interfaces. Its methods take a `postgres.DBTX` supplied by the service transaction.

## Dependencies
Uses LLM provider domain models and inputs, Postgres DBTX, UUID and the shared errors catalog.

## Run & test
Covered by `go test ./internal/llmprovider/...` with migrated Postgres testcontainers.

## Conventions
Every business query gets the tenant from context. Service transactions pass their own DBTX; repository methods do not create transactions.

## Common failures
An absent tenant context fails closed. Duplicate names and references in use return named catalog errors.
