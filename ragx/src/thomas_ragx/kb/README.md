# Knowledge Bases

## Purpose
Owns org spaces, Knowledge Bases and file storage. The org is always the App token's `oid`.

## Entry points
- `KnowledgeBaseService` provisions orgs and manages Knowledge Bases.
- `FileService` manages presigned upload (`create_upload`, `confirm_upload`), listing, deletion, retry, and download links.
- `presign.UrlSigner` signs the browser's MinIO POST policy and GET URL (`create_url_signer`, `attachment_disposition`).
- `upload.normalize_file_name` and `content_type_for` validate names and map extensions to types.
- `require_org` checks that an org space exists.
- `storage.ObjectStorage` provides MinIO access for files, including `list_objects` for the worker's sweeper.

## Dependencies
- Uses: `RegistryService` for the embedding role, `db.outbox` for ingest and purge jobs, MinIO for file storage and presigned URLs.
- Used by: the `/v1` API and the file routes.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration/test_kb_api.py
uv run pytest -q tests/integration/test_files_api.py
```

## Conventions
`PUT /v1/orgs/{orgId}` is idempotent and creates the default KB ("Mặc định") only on the first call. It needs the embedding role; without it, provisioning returns `LLM_ROLE_NOT_ASSIGNED` and creates nothing. `knowledge_bases.collection_name` (`kb.model.collection_name_for`, `kb_<embedding model id hex>_<dims>`) is written at KB creation and names the KB's Qdrant collection; it is internal and not part of `KnowledgeBaseResponse`. A KB owned by another org is always `KB_NOT_FOUND`. `fileCount` on a KB counts its non-deleted, confirmed files. The upload limit is 50 MB (`RAG_UPLOAD_MAX_BYTES` = 52428800, owner decision 2026-10-11) and the accepted extensions are pinned to `contracts/kb-upload.json` by `tests/unit/test_upload_contract.py`. The default KB cannot be deleted (`CONFLICT`). Delete marks `deleted_at` and queues `thomas.kb.purge.requested`; the worker removes objects and rows. DTOs forbid unknown fields, so a body naming an org returns `VALIDATION_FAILED`.

Ragx never receives file bytes. Upload is presign, POST, confirm (spec ragx-custom-app §14.3):
1. `POST /v1/knowledge-bases/{kbId}/uploads` with `{name, sizeBytes}` (1 to `RAG_UPLOAD_MAX_BYTES`) inserts a `kb_files` row with status `uploading` and answers `{fileId, upload: {url, fields}, expiresAt}`. The POST policy pins the key `{org}/{kb}/{file}` in `kb-raw`, a `content-length-range` of exactly `sizeBytes`, and the expiry `RAG_UPLOAD_URL_TTL` (15 min). The browser POSTs the policy fields, then the file last, straight to MinIO.
2. `POST …/files/{fileId}/confirm` checks the object: absent is `VALIDATION_FAILED` field `file` `MISSING` (the slot stays, so the browser can POST again); a different size deletes the object and answers `SIZE_MISMATCH`. Otherwise one transaction, under the KB row lock, hashes to a version and flips `uploading` to `pending` and enqueues `thomas.ingest.requested`. Duplicate content (same `sha256` in the KB) deletes the new row and object and returns the existing file. After that commit, a failing object delete is only logged (`duplicate object left for the sweep`) and confirm still answers 200; the sweeper removes the orphan later (ruling A17).
3. `GET …/files/{fileId}/download-url` answers `{url, expiresAt}`: a presigned GET valid `RAG_DOWNLOAD_URL_TTL` (60 s) that forces `Content-Disposition: attachment; filename*=UTF-8''<RFC 3986 percent-encoded NFC name>` and `application/octet-stream`, so an uploaded `.html` never renders.

An `uploading` row has no `sha256` and no `version` (a CHECK enforces it); both are set at confirm, so concurrent presigns of one name never collide on a unique index. `uploading` rows are invisible to the list, `fileCount`, retry, delete, download-url and the duplicate lookup; only confirm sees them. Confirm is idempotent by `fileId` (ruling P10): a repeat returns the same file; a folded duplicate is remembered in `idempotency_keys` (operation `confirm`) so repeating it returns the same existing file. `Idempotency-Key` (`^[A-Za-z0-9_-]{8,128}$`) is format-checked only; confirm always answers 200. Names are NFC normalized with path parts dropped and control characters rejected (field `name`, `INVALID_NAME`); the type comes from the extension list (main spec §4.1), not the client.

The signing client takes `RAG_S3_REGION` explicitly (ruling P11): minio would otherwise ask the public endpoint for the bucket region, which the ragx container cannot reach. The public endpoint scheme decides `secure`. The worker's sweeper removes expired `uploading` slots and objects no `kb_files` row owns (see the worker README). Previews exist only after ingest writes them (M1.3); `FileService.preview_size` answers `HEAD` on the preview from a `stat`. Pagination cursors are opaque base64.

## Common failures
- `ORG_NOT_PROVISIONED` → core has not provisioned the org yet.
- `LLM_ROLE_NOT_ASSIGNED` → set the embedding role in `/ui` (dev: `ragx seed-dev`).

ObjectStorage.download_to streams a raw object to a temporary path and calculates its SHA256 in the same pass for ingest integrity.

Failed streaming downloads remove the incomplete file after the writer closes and preserve the original read/write error.

ObjectStorage.put_bytes writes generated preview JSON through a worker thread; ingestion publishes the preview before marking a file ready.

The repository lock_collection includes deleted KB rows and locks before file/node mutations. Ingest publication and purge retain this lock through external point/object writes and row finalization, preventing a completed purge from being followed by orphaned vectors.
