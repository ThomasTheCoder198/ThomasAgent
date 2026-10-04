# Core OpenAPI

## Purpose

Defines the core HTTP API contract for health, authentication, the registry, and internal model resolution.

## Entry points

- `core.v1.yaml` — read by Redocly, the Postman converter, and the core route coverage test.

## Dependencies

- Uses: the shared response format and error catalog in `../errors.yaml`.
- Used by: Go core route tests, generated Postman requests, and API consumers.

## Run & test

From `contracts/`:

```bash
npm run lint:openapi
npm run gen:postman
```

## Conventions

Update the contract in the same change as every new or changed endpoint, then run `task gen` from the root.
Examples carry Postman variables. Redocly permits these example values while validating the rest of the contract.
Streaming endpoints must document `text/event-stream` and the UI Message Stream version header when introduced.

## Common failures

- Undocumented route → a Go route was added without its contract → add the path and regenerate.
- Invalid reference → a component was renamed or removed → update the reference and rerun lint.
