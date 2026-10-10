# App proxy controller

## Purpose
HTTP handler for `/api/v1/orgs/{orgId}/apps/ragx/{path}`: reads the request, calls the service, writes the app's answer unchanged.

## Entry points
- `AppProxyController.HandleForward`, registered for the six proxied methods in `cmd/core/routes.go`.

## Dependencies
- Uses: `appproxy/service`, `appproxy/model`, `platform/httpx`.
- Used by: `cmd/core`.

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run AppProxy -count=1 -p 1
```

## Conventions
The body is read through `http.MaxBytesReader(CORE_HTTP_MAX_BODY_BYTES)`; over the limit → `PAYLOAD_TOO_LARGE` before any ragx call. The response is `Cache-Control: no-store`; 204 and HEAD write no body.
