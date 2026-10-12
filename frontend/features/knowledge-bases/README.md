# features/knowledge-bases

## Purpose

The operator's Knowledge Base screens for one organization: the KB list, a KB's documents with upload, download,
retry and delete, and the stations that stand in for them while ragx has not prepared the org. Ragx owns the data;
this module only speaks to it through core. Preview UI arrives with M1.3.

## Entry points

- `contract.ts` — `RAGX_PATHS` (every KB/file URL), `kbPage`, `fileStatuses`, `FINAL_FILE_STATUSES`, the polling and paging constants and `ORG_PENDING_MAX_REFRESHES`. Types come from `@/lib/ragx/types` (generated from `contracts/openapi/ragx.v1.yaml`), never hand-written.
- `upload-rules.ts` — `checkUpload` (extension, empty, 50 MB) mirroring `contracts/kb-upload.json`; `upload-rules.test.ts` fails when they drift.
- `upload.ts` — `presignUpload`, `postToStorage`, `confirmUpload`, `fetchDownloadUrl`, `StorageUploadError`.
- `useUploadQueue.ts` — sequential queue `{ items, add, cancel, retry, dismiss }` with per-step retry.
- `useFiles.ts` — paging, `upsert` / `remove`, and polling while a file is in flight.
- `server.ts` — page loaders returning `KbLoad<T>` (`notFound`, `serviceDown`, `requestId`); `api.ts` — client mutations.
- `components/` — `KnowledgeBaseList`, `KnowledgeBaseSign`, `KnowledgeBaseDialogs` (`NewKnowledgeBaseButton`), `ConfirmDelete`, `FileList`, `FileRow`, `FileStatusMark`, `UploadRow`, `UploadDropZone`, `OrgStatusStation`, `LoadFailure`.
- Routes: `app/(workspace)/orgs/[orgId]/layout.tsx` (404 for an unknown org; `OrgStatusStation` while the org is not `ready`), `.../knowledge-bases` and `.../knowledge-bases/[kbId]`; `/knowledge-bases` redirects to the current org's list.

## Data path

Every call goes to `/api/v1/orgs/{orgId}/apps/ragx/<ragx path without /v1>` (core's generic app proxy) through the
existing `/api/v1` rewrite. All of it is JSON: no file bytes pass through Next or core, and there is no Next route
handler for uploads. Ragx deletes answer 204, which `apiFetch` resolves as `undefined`. Core forwards only
`Content-Type`, `Idempotency-Key`, `Accept-Language` and `X-Request-Id`.

## Upload: three steps, retried at the step that failed

1. **Presign** (`POST .../uploads`, `{ name, sizeBytes }`) returns a ticket: the storage URL, policy fields and `expiresAt`.
2. **Storage**: XHR `POST` of the policy `fields` (first) and `file` (last) straight to MinIO, with progress and cancel, no cookie, CSRF or request id.
3. **Confirm** (`POST .../files/{id}/confirm`) is idempotent by file id, so there is no `Idempotency-Key`.

Retry resumes where it failed: it presigns again only when there is no ticket or it has expired, posts the bytes again
when the storage step failed or confirm answered `VALIDATION_FAILED` on `file` (`MISSING`, `SIZE_MISMATCH`), and otherwise
confirms again (a confirm retry always confirms its own ticket's file, even if the ticket has expired). A row shows
Preparing, then byte progress, then Finishing. Failures: presign and confirm are `ApiError`s with a request id; the
storage step is a `StorageUploadError` with the S3 code and **no request id** (MinIO has none). Files that fail the
client check (type, size, empty) offer Dismiss only.

## Download

A button calls `GET .../download-url` through the proxy and then `window.location.assign(url)`. The URL is presigned
by ragx with `attachment` and `application/octet-stream`, so the browser saves the file without leaving the page.

## Polling and states

While any file is outside `FINAL_FILE_STATUSES` (`ready`, `failed`, `degraded`) and the tab is visible, the first page is
re-read every `FILE_POLL_INTERVAL_MS`. A failed poll keeps the last rows and shows an inline notice with the request id.
`degraded` shows the KB ring like `ready`. "Service unavailable" is decided by `status >= 500`, never by the code
`RAGX_UNAVAILABLE` (core shows 5xx as `INTERNAL_ERROR`).

Org stations: `pending` keeps the Thinking Orb and refreshes at most `ORG_PENDING_MAX_REFRESHES` times (about a minute),
then shows a static ring and the stalled copy; `failed` shows the code in mono. Both always offer Retry, which calls
`retryOrganization` (synchronous) and refreshes.

## Dependencies

- Uses: `features/orgs` (`getOrganization`, `retryOrganization`, `orgHome`), `features/shell`, `components/ui`, `components/metro`, `lib/api`, `lib/ragx`, `lib/validation`.
- Used by: the `app/(workspace)/orgs/[orgId]` routes and the sidebar's Knowledge Base entry.

## Run & test

```bash
cd frontend
npx vitest run features/knowledge-bases lib/api
npm run dev:mock   # fake core with the proxy routes and a fake S3 on :8787
npm run e2e -- e2e/knowledge-bases.spec.ts
```

## Common failures

- The upload row fails with `NETWORK` / status 0 → the browser could not reach MinIO (CORS or the public endpoint) → check `MINIO_API_CORS_ALLOW_ORIGIN` and `RAG_S3_PUBLIC_ENDPOINT`.
- The upload row shows `EntityTooLarge` or `EntityTooSmall` → the file's size changed after presign, or `sizeBytes` was edited → pick the file again.
- Confirm says `MISSING` → the bytes never reached storage; Retry posts them again.
