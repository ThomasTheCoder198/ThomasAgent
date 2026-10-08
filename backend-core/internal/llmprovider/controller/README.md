# Registry handler

## Purpose
Owns public provider/model/role routes and the service-token protected model resolver route.

## Entry points
`NewRegistryHandler`, `Mount`, `MountInternal`, `RequireServiceToken`.

## Dependencies
Consumes registry service interfaces, DTOs and shared HTTP response/error helpers.

## Run & test
Covered by `go test ./internal/registry/...` and the generated API collection.

## Conventions
Public responses map domain models to DTOs. The resolver response may contain a provider key only behind the internal service token.

## Common failures
Invalid bearer tokens return the named INTERNAL_TOKEN_INVALID error.
