# mock-core

## Purpose

A local fake of Go core so the web shell and chat surface can be built and reviewed before M2 ships.
It speaks core's real wire format: the `{ data, meta }` / `{ error }` envelopes, catalog error codes and
messages from `contracts/errors.yaml`, the `thomas_session` + `thomas_csrf` cookies with CSRF checks on
unsafe methods, and the AI SDK UI Message Stream (`x-vercel-ai-ui-message-stream: v1`) for chat.
All content it serves is **synthetic**.

Error responses echo a valid `X-Request-Id` in both the header and `error.requestId`; missing or invalid IDs are replaced.

## Entry points

- `server.ts` — `node:http` router; run with `npm run mock-core` (Node 24 strips the TypeScript types).
- `auth.ts` — login / me / logout, in-memory sessions.
- `catalog.ts` — models and role assignments in the documented M0.2 shapes.
- `conversations.ts` — in-memory conversation store; the seeded refund conversation is replayed from the same script the live chat streams.
- `chat/` — `POST /api/v1/chat`: phase one (reasoning → KB search → read → sources → answer → Linear approval request), phase two after the owner approves or denies.
- `agents.ts` — `GET /api/v1/agents`, `GET|PATCH /api/v1/agents/{id}`, `GET /api/v1/knowledge-bases` (in-memory). Patches are checked like core must: another org's KB → `FORBIDDEN`, unknown KB or model id → `VALIDATION_FAILED`.
- `orgs.ts` — `GET|POST /api/v1/orgs`, `GET|PATCH /api/v1/orgs/{id}`, `POST /api/v1/orgs/{id}/provision`, in memory and synchronous like core: create answers 201 `ready` at once (a name ending `!fail` answers 201 `failed` with `RAGX_UNAVAILABLE`; one ending `!role` answers 201 `failed` with `LLM_ROLE_NOT_ASSIGNED`, and Retry then succeeds; one ending `!down` stays `ready` but every ragx proxy route answers 503 with body `INTERNAL_ERROR` + requestId, like core with ragx down); a duplicate name (ignoring case) → 409 `CONFLICT` with `details.fields`; PATCH only changes the row; provision answers 200 `ready`. Seeds **Công ty Demo** (ready), **Beta** (failed, `LLM_ROLE_NOT_ASSIGNED`) and **Gamma** (a stuck `pending`). Exports `findOrg(id)` for the Knowledge Base proxy routes.
- `knowledge-bases.ts` — core's app proxy to ragx, `/api/v1/orgs/{orgId}/apps/ragx/<ragx path>` for GET, HEAD, POST, PUT, PATCH and DELETE, dispatched like ragx would: KBs (`GET|POST knowledge-bases`, `GET|PATCH|DELETE knowledge-bases/{kb}`; deleting the default KB is `CONFLICT`, deletes answer **204**), files (`GET .../files?cursor&limit`, `POST .../files/{id}/retry`, `DELETE .../files/{id}`) and the upload flow (`POST .../uploads` presigns, `POST .../files/{id}/confirm` is idempotent and answers `VALIDATION_FAILED` `file`/`MISSING` without the bytes, `GET .../files/{id}/download-url`). An unknown org is `NOT_FOUND`, an org that is not ready `ORG_NOT_PROVISIONED`, a KB `KB_NOT_FOUND`, and a rest starting with `orgs/` stands for core's 403 to 404 mapping. The demo org seeds a second KB and three files (ready, parsing, failed `INGEST_PARSE_FAILED`); each list call moves an in-progress file one step. Handlers are named `ragx...`.
- `fake-s3.ts` — a public fake of MinIO's `kb-raw` bucket outside `/api/v1`: `OPTIONS` and `POST /__mock/s3/kb-raw` (CORS for `MOCK_CORE_WEB_ORIGIN`, the policy's `content-length-range` is checked: `EntityTooLarge` / `EntityTooSmall` as XML) and `GET /__mock/s3/kb-raw/<key>` (the bytes as an attachment with `application/octet-stream`). Ticket and download URLs point at `MOCK_CORE_PUBLIC_URL` (default `http://localhost:<port>`).
- `POST /__mock/reset[?agent=id]` — fake-core only (outside `/api/v1`, so the web app cannot reach it). Unscoped, it restores every Agent, the org fixtures, the KBs, files and stored objects. With `?agent=id` (the Agents e2e, per test) it restores only that Agent, because other specs run in parallel against the same fake.
- `fixtures/` — the synthetic refund-policy documents and citations, and three synthetic Agents with their KBs and tool sources.

## Dependencies

- Uses: `ai` (stream helpers), `frontend/features/chat/contract.ts` (provisional M2 shapes), `frontend/lib/errors/codes.gen.ts`.
- Used by: `npm run dev:mock`, Playwright e2e in mock mode. Excluded from the Docker image.

## Run & test

```bash
npm run dev:mock            # fake core on :8787 + next dev with WEB_CORE_URL=http://localhost:8787
npm run mock-core           # fake core alone
```

Sign in with `owner@thomas.local` / `metro-wayfinding` (override with `MOCK_CORE_OWNER_EMAIL` / `MOCK_CORE_OWNER_PASSWORD`).
`MOCK_CORE_SPEED=0` makes scripted runs instant; `MOCK_CORE_PORT` moves the port (never 8080, which belongs to real core).

## Switching to real core

Nothing in `app/`, `features/` or `lib/` knows this module exists. Point `WEB_CORE_URL` at core
(`http://localhost:8080`, or `http://core:8080` in compose) and run `npm run dev`. The provisional
endpoints (`/api/v1/chat`, `/chat/scope`, `/conversations`, `/documents/{id}/pages/{n}`, `/agents`,
`/knowledge-bases`) must be
specified in `contracts/openapi/core.v1.yaml` during M2; update `features/chat/contract.ts` to the
generated types and delete this module when core serves them.

## Conventions

Follow the root `AGENTS.md`. Every timing lives in `config.ts`; every scripted string in `fixtures/`.

## Common failures

- `ERR_UNSUPPORTED_TYPESCRIPT_SYNTAX` → strip-only mode rejects enums, namespaces and parameter properties → use plain fields and `as const` objects.
- Port 8787 in use → an older fake core is still running → stop it or set `MOCK_CORE_PORT`.
