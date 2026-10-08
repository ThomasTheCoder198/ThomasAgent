# Registry DTO

## Purpose
Owns provider, model and role HTTP request/response shapes.

## Entry points
Request and response structs, request-to-domain conversions and domain-to-response conversions.

## Dependencies
Depends on registry models and UUID. Consumed by registry HTTP handlers.

## Run & test
Covered by `go test ./internal/registry/...` and the OpenAPI contract tests.

## Conventions
Only DTOs carry JSON field names. Provider responses expose `hasApiKey`, never the key.

## Common failures
Changing a DTO field must be coordinated with `contracts/` and code generation.
