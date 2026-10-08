# Registry service

## Purpose
Owns registry business orchestration and defines the behaviors required by HTTP handlers.

## Entry points
`Service`, `NewRegistryService`, `RegistryService` and `Resolver` interfaces.

## Dependencies
Depends on registry models, repository, catalog interface, vault and audit. HTTP DTOs are converted by the handler.

## Run & test
Covered by `go test ./internal/registry/...`.

## Conventions
Public API and internal resolver depend only on the methods they consume. Implementation owns validation, transaction and catalog orchestration.

## Common failures
An unassigned role returns the shared named error; secret keys stay out of public responses.
