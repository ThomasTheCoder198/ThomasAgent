# frontend/lib/ragx

Ragx's API types for the frontend.

- `api.gen.ts` is generated from `contracts/openapi/ragx.v1.yaml` by `npm run gen:ragx-types` in `contracts/` (part of
  `task gen`). Never edit it; it is excluded from prettier and eslint.
- `types.ts` names the schemas the Knowledge Base screens use as aliases of `components["schemas"][...]` (rule 3: no
  hand-written copies).
- `types.test.ts` pins the aliases with `expectTypeOf`; `bun run typecheck` enforces it.

The browser never calls ragx directly: requests go through core at `/api/v1/orgs/{orgId}/apps/ragx/...`. File bytes go
straight to MinIO with the presigned URL from `UploadTicket`.

The generated schema includes all four M1.3 KB tools and typed manifest definitions. These generated types also describe the claim-scoped tool API; browser access continues through core.
