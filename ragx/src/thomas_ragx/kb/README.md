# Knowledge Bases

## Purpose
Owns org spaces, Knowledge Bases and file storage. The org is always the App token's `oid`.

## Entry points
- `KnowledgeBaseService` provisions orgs and manages Knowledge Bases.
- `FileService` manages upload, listing, deletion, retry, and download.
- `receive_upload` validates and spools one multipart file.
- `require_org` checks that an org space exists.
- `storage.ObjectStorage` provides MinIO access for files.

## Dependencies
- Uses: `RegistryService` for the embedding role, `db.outbox` for purge jobs, MinIO for file storage.
- Used by: the `/v1` API and the file routes.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration/test_kb_api.py
uv run pytest -q tests/integration/test_files_api.py
```

## Conventions
`PUT /v1/orgs/{orgId}` is idempotent and creates the default KB ("Mặc định") only on the first call. It needs the embedding role; without it, provisioning returns `REGISTRY_ROLE_NOT_ASSIGNED` and creates nothing. A KB owned by another org is always `KB_NOT_FOUND`. The default KB cannot be deleted (`CONFLICT`). Delete marks `deleted_at` and queues `thomas.kb.purge.requested`; the worker removes objects and rows. DTOs forbid unknown fields, so a body naming an org returns `VALIDATION_FAILED`.

Uploads need `Content-Length` and are refused before parsing above `RAG_UPLOAD_MAX_BYTES + RAG_UPLOAD_MULTIPART_OVERHEAD_BYTES`. The actual multipart bytes are bounded by the same cap while Starlette parses them. Starlette spools the single file part to a temp file; ragx hashes it and writes `kb-raw/{org}/{kb}/{file}`. Names are NFC normalized with path parts dropped, and control characters are rejected; the type comes from the extension list (main spec §4.1). The KB row lock serializes uploads into one KB, so same-name versions and same-content duplicates never hit a unique violation; a duplicate returns the existing file with 200. `Idempotency-Key` (`^[A-Za-z0-9_-]{8,128}$`) binds that result, including a duplicate, for `RAG_IDEMPOTENCY_TTL_SECONDS`. A key whose file has since been deleted is dropped and the upload proceeds as new; the same key on another KB is `CONFLICT`. Raw downloads use RFC 5987 `filename*`. Previews exist only after ingest writes them (M1.3). Pagination cursors are opaque base64.

## Common failures
- `ORG_NOT_PROVISIONED` → core has not provisioned the org yet.
- `REGISTRY_ROLE_NOT_ASSIGNED` → set the embedding role in `/ui` (dev: `ragx seed-dev`).
