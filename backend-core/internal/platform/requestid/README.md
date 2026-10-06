# Request id

## Purpose
Owns the request id format shared by core, ragx and the frontend: the `X-Request-Id` header name, the
accepted pattern and the `req_`/`job_` generators. It does not store ids; the context helpers live in
`internal/platform/logging`.

## Entry points
- `IsValid` — `httpx` checks incoming headers; `jobs` checks ids read from stream entries.
- `NewRequest` — `httpx` when a header is missing or rejected.
- `NewJob` — `jobs.EnqueueOutbox` and scheduled work when nothing started from a request.

## Dependencies
- Uses: `github.com/google/uuid`.
- Used by: `internal/platform/httpx`, `internal/jobs`, `cmd/core`.

## Run & test
```bash
cd backend-core
go test ./internal/platform/requestid -count=1
```

## Conventions
The single definition is `contracts/request-id.json`. Go, Python and TypeScript copy the values and each
has a test that reads that file, so editing the contract without the services turns tests red.
See the [glossary](../../../../docs/glossary.md) for request id and idempotency key.

## Common failures
- `TestContract_MatchesSharedDefinition` fails → the contract changed → update the constants here, in
  `ragx/src/thomas_ragx/platform/request_id.py` and in `frontend/lib/api/request-id.ts`.
