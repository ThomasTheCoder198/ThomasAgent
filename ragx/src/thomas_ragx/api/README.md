# RAG API

## Purpose
Owns FastAPI construction, request tracing, error envelopes, and health endpoints.

## Entry points
- `app.py:create_app`: application factory.
- `health.py`: GET /healthz.

## Dependencies
- Uses: platform settings, errors, logging, and tracing.
- Used by: CLI serve command and API tests.

## Run & test
```bash
cd ragx
uv run ragx serve
uv run pytest -q tests/unit
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
