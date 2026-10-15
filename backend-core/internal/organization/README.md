# Organization

## Purpose

Owns the organizations the operator manages and provisions each one into ragx synchronously. It does not own Knowledge Bases or the app proxy: those use `RequireOrg` from here to learn which org a request is for.

## Entry points

The domain follows the same layers as every core domain: `controller → service → repository` (and a ragx call through `platform/ragx`), with `model`, `dto` and `middleware` beside them.

| Folder | Holds | Example |
|---|---|---|
| `controller/` | HTTP handlers only | `OrganizationController.HandleCreateOrganization` |
| `dto/` | JSON request and response types | `CreateOrganizationRequest`, `OrganizationResponse` |
| `service/` | Rules, transactions and the ragx sync | `OrganizationService`, `RagxProvisioner` |
| `repository/` | SQL only | `OrganizationRepo` |
| `model/` | Domain types and the name rule | `Organization`, `Settings`, `NormalizeName` |
| `middleware/` | `RequireOrg` | `RequireOrg`, `OrganizationFromContext` |

Routes: `GET`/`POST /api/v1/orgs`, `GET`/`PATCH /api/v1/orgs/{orgId}`, `POST /api/v1/orgs/{orgId}/provision`.

## Dependencies

- Uses: `platform/ragx` (`Caller.ProvisionOrg`), `platform/retry`, `platform/audit`, `platform/postgres`, `platform/tenant`, `platform/httpx`, named catalog errors.
- Used by: `cmd/core` (routes and the composition root); the app proxy (`internal/appproxy`) through `RequireOrg`.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/organization/... ./tests/integration -count=1 -p 1
```

The HTTP flow with a fake ragx that verifies the App token lives in `tests/integration/organization_http_test.go`.

## Conventions

- Organizations are platform data: rows carry `tenant_id = 'default'`. The id is a core-generated UUID and is never `default` (a CHECK enforces it). The id is the tenant id of the org's business data and ragx's `org_id`.
- Provisioning is synchronous: there is no outbox job and no worker. Create inserts `pending`, commits, then calls ragx; a ragx error becomes the org's state (`failed` + `ragxErrorCode`), not the call's error, so create always answers 201 and `POST /provision` always answers 200 with the real state (ruling P1).
- `PATCH` of a non-`ready` org only writes the row; Retry (`POST /provision`) sends the current values (P2). `PATCH` of a `ready` org syncs at once; a failed sync keeps it `ready` with `settingsSynced = false` and the code.
- The row starts at `settings_version = 1` and increments on each real name or parsing change. Core sends that version with each ragx PUT. It marks `settingsSynced` true only when ragx returns the row's current version; a late result for an older version cannot claim the newer edit. An unchanged PATCH writes nothing. A successful PUT still marks the org `ready`.
- Provisioning runs on a context detached from the request's cancellation, bounded per attempt by `CORE_RAGX_REQUEST_TIMEOUT`, so a closed browser tab still records the outcome (P4). Attempts: `CORE_RAGX_PROVISION_ATTEMPTS`; only retryable errors are retried.
- `RequireOrg` is the only place a path org becomes tenant context (AGENTS.md rule 11).
- The name limit (200 characters) is repeated in Go, OpenAPI, ragx and the frontend; each side has a test that pins it.

## Common failures

- `failed` with `LLM_ROLE_NOT_ASSIGNED` → ragx has no embedding role → assign it in the ragx UI, then Retry.
- `failed` with `RAGX_UNAVAILABLE` → ragx unreachable or misconfigured (a rejected App token also maps here) → check `CORE_RAGX_URL` and the signing key, then Retry.
- `pending` for long → the process died between commit and the ragx call → Retry.
