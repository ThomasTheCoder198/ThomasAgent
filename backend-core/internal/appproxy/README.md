# App proxy

## Purpose

The generic data plane between the browser and an app (today only ragx): `/api/v1/orgs/{orgId}/apps/ragx/{path}` forwards to `{CORE_RAGX_URL}/v1/{path}` with a short-lived App token for that org. Core never learns the app's routes (spec §14.2). M3a reuses this domain for Custom Apps. It does not own organizations (`RequireOrg` comes from `internal/organization`) or the ragx client (`internal/platform/ragx`).

## Entry points

| Folder | Holds | Example |
|---|---|---|
| `controller/` | Reads the request (body capped by `CORE_HTTP_MAX_BODY_BYTES`), writes the app's answer back | `AppProxyController.HandleForward` |
| `service/` | Path, method and `Idempotency-Key` checks; picks the scope | `AppProxyService`, `Forwarder` |
| `model/` | `AppCall`, `AppResponse`, the method-to-scope table | `ForwardedMethods`, `ScopeFor` |

Middleware order on the route: `LimitRequestDuration` → `RequireSession` (CSRF on unsafe methods) → `RequireOrg` → the proxy.

## Dependencies

- Uses: `platform/ragx` (`Caller.Forward`), `platform/httpx` (header names, `ValidIdempotencyKey`), `platform/apptoken` (scopes), named catalog errors.
- Used by: `cmd/core` (routes and composition root).

## Run & test

```bash
cd backend-core
go test ./internal/appproxy/... -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run AppProxy -count=1 -p 1
```

## Conventions

- Core never learns app routes: the path is forwarded, so a new ragx route needs no core change.
- Scope by method (one table, `model.methodScopes`, drives both the registered methods and the scope):

| Method | Scope |
|---|---|
| `GET`, `HEAD` | `kb:read` |
| `POST`, `PUT`, `PATCH`, `DELETE` | `kb:write` |

  Other methods are not registered (`OPTIONS` would skip CSRF), so gin answers 405. The proxy token never carries `kb`; `oid` is the org `RequireOrg` loaded.
- `{path}` (gin's leading `/` removed) must match `^[A-Za-z0-9._~/-]+$`, contain no `..`, no `.` segment and no `//`, be non-empty and not end with `/`; otherwise `NOT_FOUND` with no ragx call.
- Only `Content-Type`, `Idempotency-Key` (`^[A-Za-z0-9_-]{8,128}$`, else `VALIDATION_FAILED` on field `Idempotency-Key`), `Accept-Language` and core's `X-Request-Id` are forwarded; never cookies or the browser's `Authorization`.
- Error table (ragx answer → core answer). The path, scope and key checks live in `service/`; the table and the response cap (`CORE_RAGX_MAX_RESPONSE_BYTES`) live in `ragx.Caller.Forward`.

| Ragx answers | Core answers |
|---|---|
| 2xx, or 4xx whose catalog code has exactly that status | verbatim status, `Content-Type` and body |
| `401` | `RAGX_UNAVAILABLE` 503 (key misconfiguration) |
| `403` | `NOT_FOUND` 404 (the route needs a scope the UI never gets) |
| 1xx/3xx/5xx, unknown code, code/status drift, timeout, unreachable, body over the cap | `RAGX_UNAVAILABLE` 503 |

  5xx bodies read `INTERNAL_ERROR` + `requestId`; `RAGX_UNAVAILABLE` shows only in the status and the boundary log.
- Responses carry `Cache-Control: no-store` and `X-Content-Type-Options: nosniff` (the app's Content-Type is served from core's origin); a 204 or a HEAD answer writes no body.
- One `ragx call` log line per request (method, route, status, `duration_ms`, `request_id`, `jti`), never the token or a body.

## Common failures

- 503 on every call with a `401` cause in the log → `CORE_APP_SIGNING_KEY_ID` or the key does not match ragx's `RAG_CORE_PUBLIC_KEYS` → fix the key pair.
- 409 `ORG_NOT_PROVISIONED` → the org is not `ready` → Retry the provision.
- 404 on a ragx route that exists → it needs `org:provision` or `tool:*`, which the UI never gets.
