# Contracts

## Purpose

Single source for error codes and shared schemas used by the Go core, Python RAG app, and frontend. Owns catalog validation, core OpenAPI, deterministic Postman generation, and route drift checks in core tests.

## Entry points
- `errors.yaml` — authoritative error codes, HTTP statuses, retryability, and VI/EN messages.
  Includes authentication and LLM provider catalog errors (`INTERNAL_TOKEN_INVALID` stays registered but is unused since M1.2) and the M1.3 ingest and retrieval codes (`INGEST_CONTENT_CHANGED`, `INGEST_PARSE_FAILED`, `INGEST_NO_TEXT`, `INGEST_EMBEDDING_MISMATCH`, `KB_NODE_NOT_FOUND`).
- `request-id.json` — the `X-Request-Id` header, its pattern and prefixes, with valid/invalid examples. There is no
  generator: Go (`backend-core/internal/platform/requestid`), Python (`ragx/.../platform/request_id.py`) and
  TypeScript (`frontend/lib/api/request-id.ts`) copy the values and each has a test that reads this file.
- `app-token.json` — generated App-token test vector (trusted key, valid tokens, invalid tokens with the rejection
  reason). Built by `cmd/apptokengen` from `internal/apptoken`; ragx's verifier and core's signer test against it.
- `kb-upload.json` — Knowledge Base upload limits and accepted extensions (`maxFileBytes`, 50 MB since 2026-10-11, and `extensions`). Ragx enforces them; the frontend validates
  with them. Each side has a test that fails when it drifts.
- `openapi/ragx.v1.yaml` — the ragx `/v1` API that only core calls (App token per request). File bytes never pass through it: upload is presign (`createUpload`), a browser POST to MinIO, then `confirmUpload`; download is `getDownloadUrl`. `HEAD` on a GET route is served but not documented. Tool routes are added in M1.3.
- `npm run gen:ragx-types` — `openapi-typescript` turns `openapi/ragx.v1.yaml` into `frontend/lib/ragx/api.gen.ts` (run by `task gen`, never edited). It lives here, not in `frontend/`, because its peer dependency `typescript ^5` conflicts with the frontend's TypeScript 6; `typescript@5.9.3` is pinned in this package only.
- `cmd/errorcodegen` — command invoked by `task gen`.
- `internal/errorcodegen` — catalog parser and Go, Python, and TypeScript renderers.
- `openapi/core.v1.yaml` — source of truth for the core HTTP API.
- `postman/postprocess.mjs` — adds collection scripts and orders the generated request flow.

## Dependencies
- Uses: `gopkg.in/yaml.v3` v3.0.1; tests use `github.com/stretchr/testify` v1.12.1.
- Tooling: Node 24.21.0; pinned Redocly CLI, openapi-to-postmanv2, and Newman in `package.json`.
- Used by: `backend-core`, `ragx`, and `frontend` through generated catalogs.

## Run & test

Install contract tooling once with `npm ci` from this folder. From the repository root, with Go 1.27.1 and Node 24.21.0 on PATH:

```bash
task gen
task test:contracts
task api-test
```
For a fresh test run from this folder:
```bash
go mod tidy
go test ./... -count=1
go vet ./...
```

## Conventions

OpenAPI is the source of truth for the core API. `task gen` validates it and regenerates the Postman collection; never edit the collection by hand. `task api-test` runs it with Newman.
Both OpenAPI files are linted by `task gen`. Ragx's FastAPI routes are checked against `ragx.v1.yaml` by `ragx/tests/unit/test_openapi_contract.py`.
The login contract documents all three attempt budgets, the intentional email-global lockout and 429 `Retry-After`. Session authentication uses database expiry time. These hardening changes preserve request/response schemas and error codes.
JSON request bodies require `Content-Type: application/json` (charset parameters are allowed).
Missing, malformed or other content types return catalog `UNSUPPORTED_MEDIA_TYPE` (415).
Unknown fields and malformed JSON return `VALIDATION_FAILED` (400). The stricter body
decoding is a breaking change for callers that previously sent other media types or extra fields.
API tests require a running core and owner credentials. Export `CORE_AUTH_OWNER_EMAIL` and `CORE_AUTH_OWNER_PASSWORD` to use a disposable test server, or fall back to the local compose environment. `CORE_API_TEST_BASE_URL` overrides the default `http://localhost:8080`.
The API test assigns `chat.fast` and verifies that deleting its model/provider returns conflict; use a disposable development database. The programmatic runner passes credentials through environment values and prints only assertion counts and failed check names.
Never edit generated files. Add an error code to `errors.yaml` first, then run `task gen`.
Catalog version is 1; codes use UPPER_SNAKE and must be unique. HTTP statuses are 400–599 and both localized messages are required.
The parser sorts entries by code so generated output is deterministic. Renderers emit UTF-8 with LF line endings.
Three files are generated: `backend-core/internal/errors/errors.go` (canonical Go definitions),
`ragx/src/thomas_ragx/platform/error_codes_gen.py`, and `frontend/lib/errors/codes.gen.ts`.
The canonical Go file defines immutable named errors such as `errors.ErrNotFound` and
maps codes to `ErrorDefinition{HTTPStatus, Retryable, MessageVI, MessageEN}` through `LookupDefinition`.
There is no second Go message catalog or mutable global error instance.
Python exposes `ErrorDefinition.http_status` and `ERROR_DEFINITIONS`; TypeScript exposes
`errorDefinitions` with `httpStatus`. See the [glossary](../docs/glossary.md) for shared terminology.

## Common failures
- Invalid catalog → malformed YAML, duplicate code, unsupported version, invalid status, or missing message → correct `errors.yaml` and rerun generation.
- Golden mismatch → output differs from the fixtures → inspect the diff; regenerate an intentional change with the generator tests described in [the renderer README](internal/errorcodegen/README.md).
- Missing dependency checksum → dependencies have not been resolved → run `go mod tidy`.
- Missing Node tooling → contract dependencies have not been installed → run `npm ci` in this folder.
- Newman expected-status failure → an endpoint failed the authenticated flow → inspect its status and the core trace; do not accept an unauthorized response as success.
