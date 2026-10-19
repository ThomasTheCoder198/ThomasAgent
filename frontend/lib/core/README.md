# frontend/lib/core

Core's M2 API types for the frontend.

- `api.gen.ts` is generated from `contracts/openapi/core.v1.yaml` by `npm run gen:core-types` in `contracts/` (part
  of `task gen`). Never edit it; it is excluded from prettier and eslint.
- `types.ts` names the schemas the Agents, chat and Settings screens use (rule 3: no hand-written copies).
- `types.test.ts` pins the aliases with `expectTypeOf`; `npm run typecheck` enforces it.

Four aliases are still hand-written, in one marked block at the end of `types.ts` (ruling F2c1-1): `ChatRequest`,
`SteerRequest`, `RunUsage` and `CitationLocatorWire`. Core's OpenAPI does not describe the chat endpoints yet (Tasks
2b-4/2b-5 add them). Their shapes follow the index "Wire contracts"; replace each with a `Schemas[...]` alias as
soon as the schema appears in `core.v1.yaml`. `CitationLocatorWire.fileVersion` is numeric, matching ragx's file
version and the preview comparison.

The M0.2 registry types in `features/registry/types.ts` are still hand-written: their OpenAPI properties are all
optional, and aliasing them would ripple through every screen. They move here once those properties are marked
required in the contract.
