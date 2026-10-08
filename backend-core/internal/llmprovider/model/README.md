# LLM provider model

## Purpose
Owns provider, model, capability and role domain types.

## Entry points
`Provider`, `Model`, `ProviderInput`, `ModelInput`, `RoleAssignment`, `ResolvedModel` and their typed enums.

## Dependencies
Go time and UUID only. Used by LLM provider DTOs, repositories and services.

## Run & test
Covered by `go test ./internal/llmprovider/...`.

## Conventions
Domain models are independent of HTTP decoding and request validation.

## Common failures
Role capability requirements must match the model LLM provider rules.
