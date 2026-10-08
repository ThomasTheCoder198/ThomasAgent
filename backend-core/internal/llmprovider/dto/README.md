# LLM provider DTO

## Purpose
Owns provider, model and role HTTP request/response shapes.

## Entry points
Request and response structs, request-to-domain conversions and domain-to-response conversions.

## Dependencies
Depends on LLM provider models and UUID. Consumed by LLM provider HTTP controllers.

## Run & test
Covered by `go test ./internal/llmprovider/...` and the OpenAPI contract tests.

## Conventions
Only DTOs carry JSON field names. Provider responses expose `hasApiKey`, never the key.

## Common failures
Changing a DTO field must be coordinated with `contracts/` and code generation.
