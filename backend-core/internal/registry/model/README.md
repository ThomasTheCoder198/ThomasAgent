# Registry model

## Purpose
Owns provider, model, capability and role domain types.

## Entry points
`Provider`, `Model`, `ProviderInput`, `ModelInput`, `RoleAssignment`, `ResolvedModel` and their typed enums.

## Dependencies
Go time and UUID only. Used by registry DTOs, repositories and services.

## Run & test
Covered by `go test ./internal/registry/...`.

## Conventions
Domain models are independent of HTTP decoding and request validation.

## Common failures
Role capability requirements must match the model registry rules.
