# features/registry

## Purpose

Typed mirrors of the M0.2 registry schemas (models, role assignments) the web app reads today; the composer's
model picker uses them so no model is hardcoded. Provider/model management screens arrive with Settings (M2).

## Entry points

- `types.ts` — `Model`, `RoleAssignment`, `ModelChoice`, `REGISTRY_PATHS`, `DEFAULT_CHAT_ROLE`.

## Dependencies

- Uses: `contracts/openapi/core.v1.yaml` (by hand until an OpenAPI type generator lands).
- Used by: `features/chat`, `mock-core/catalog.ts`.
