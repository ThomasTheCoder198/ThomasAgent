# LLM provider service

## Purpose
Owns LLM provider business orchestration and defines the behaviors required by HTTP controllers.

## Entry points
`Service`, `NewLLMProviderService`, `LLMProviderService` and `Resolver` interfaces.

## Dependencies
Depends on LLM provider models, repository, catalog interface, vault and audit. HTTP DTOs are converted by the controllers.

## Run & test
Covered by `go test ./internal/llmprovider/...`.

## Conventions
Public API and internal resolver depend only on the methods they consume. Implementation owns validation, transaction and catalog orchestration.

## Common failures
An unassigned role returns the shared named error; secret keys stay out of public responses.
