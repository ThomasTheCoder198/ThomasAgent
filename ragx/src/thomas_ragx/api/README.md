# RAG API

## Purpose
Owns FastAPI construction, request tracing, error responses, and health endpoints.

## Entry points
- `app.py:create_app`: application factory.
- `health.py`: GET /healthz and GET /readyz.
- `orgs.py`: org provisioning.
- `knowledge_bases.py`: Knowledge Base CRUD.
- `files.py`: `POST /v1/knowledge-bases/{kbId}/uploads` (presign), `POST …/files/{fileId}/confirm`, `GET …/files/{fileId}/download-url`, file listing, deletion, retry and preview. `HEAD` on the preview answers from a `stat` (status and `Content-Length`, no read of the object).
- `routing.py`: `api_router`, whose routes answer `HEAD` like `GET`.
- `manifest.py`: app manifest.
- `services.py`: dependency access to application services.

## Dependencies
- Uses: platform config, errors, logging, and tracing.
- Used by: CLI serve command and API tests.

## Run & test
```bash
cd ragx
uv run ragx serve
uv run pytest -q tests/unit
```

## Conventions
See the [RAG service conventions](../../../README.md#conventions) for Config, credentials, shared naming, and boundary logging.

Every GET route also serves `HEAD` (`HeadCapableRoute`; FastAPI does not do it): same status and headers, no body. HEAD has no body on the wire, so a failed HEAD carries only a status. HEAD is not in the OpenAPI contract, which documents GET once.

Every `/v1` route takes a scoped `Caller` from `auth.dependencies`; services read `app.state.*_service` through `api/services.py`.
`ragx/tests/unit/test_openapi_contract.py` keeps served routes and `contracts/openapi/ragx.v1.yaml` identical.

The application passes `Config.environment` to logging configuration, preserving the `env` log field. Middleware binds only the request ID; handler logs inherit it and the active request span, including synchronous handlers. Logging fields follow the [shared contract](../../../../backend-core/internal/platform/logging/README.md#log-field-contract). `/readyz` checks Postgres, Redis and MinIO with `RAG_READINESS_TIMEOUT_SECONDS` each; failures are logged by name and the 503 body shows only `INTERNAL_ERROR`. `create_app` takes `Resources`; tests pass `tests/support.FakeResources`.

The access log is one `request completed` line per request: method, route (FastAPI pattern or `unmatched`), status, integer `duration_ms`, `request_id`, and `org_id`/`jti` when an App token was accepted (read from `request.state` after the handler). Level: info 2xx/3xx, warning 4xx, error 5xx. The root span gets `org_id`.

The root span `rag.http.request` carries `request_id` and `openinference.span.kind=CHAIN`. Every response, including errors, carries `X-Ragx-Trace-Id` (32 lowercase hex characters) so core can log it on `tool.call` and Grafana can open the trace in Phoenix (`/redirects/traces/<id>`). `org_id` comes from the authenticated caller state, never from parameters. The app uses its own tracer provider; only the serve CLI installs it globally. Phoenix export failures do not block requests.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.

`X-Request-Id` is accepted only when it matches the shared pattern. Otherwise the middleware generates a new `req_` id; the rejected value is never logged. Completion logs include monotonic elapsed `duration_ms` and add `request_id_source` for generated ids and `client_request_id_rejected` when a client value was rejected.
