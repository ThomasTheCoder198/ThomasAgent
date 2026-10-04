# Frontend error catalog

## Purpose
Provides the generated shared error definitions for frontend consumers.

## Entry points
- `codes.gen.ts`: `ErrorCode` and `errorDefinitions` with `httpStatus`, retryability, and localized messages.
  Includes authentication, internal service-token validation, and provider/model registry errors.
- [Glossary](../../../docs/glossary.md): shared error and response terminology.

## Dependencies
- Uses: contracts/errors.yaml through the Go catalog generator.
- Used by: frontend API error handling as it is implemented.

## Run & test
```bash
task gen
task test:contracts
```

## Conventions
Never edit generated code. Register codes in contracts/errors.yaml and regenerate.

## Common failures
- Missing code: register it in the catalog and run task gen.
- Stale output: run task gen and review the generated diff.
