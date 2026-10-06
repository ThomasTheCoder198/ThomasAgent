# RAG API

## Purpose
Owns FastAPI construction, request tracing, error responses, and health endpoints.

## Entry points
- `app.py:create_app`: application factory.
- `health.py`: GET /healthz.

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

The application passes `Config.environment` to logging configuration, preserving the `env` log field. Middleware binds only the request ID; handler logs inherit it and the active request span, including synchronous handlers. Logging fields follow the [shared contract](../../../../backend-core/internal/platform/logging/README.md#log-field-contract).

The root span `rag.http.request` carries `request_id` and `openinference.span.kind=CHAIN`. Every response, including errors, carries `X-Ragx-Trace-Id` (32 lowercase hex characters) so core can log it on `tool.call` and Grafana can open the trace in Phoenix (`/redirects/traces/<id>`). `org_id` is added with authentication in the ragx custom-app spec, never from parameters. The app uses its own tracer provider; only the serve CLI installs it globally. Phoenix export failures do not block requests.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.

`X-Request-Id` is accepted only when it matches the shared pattern. Otherwise the middleware generates a new `req_` id; the rejected value is never logged. Completion logs include monotonic elapsed `duration_ms` and add `request_id_source` for generated ids and `client_request_id_rejected` when a client value was rejected.
