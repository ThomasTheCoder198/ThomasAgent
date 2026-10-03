# RAG tests

## Purpose
Owns regression coverage for the Python service foundations.

## Entry points
- `unit/`: platform, API, and bootstrap regression tests.

## Dependencies
- Uses: pytest, service code, and test fakes.
- Used by: task test:ragx and CI.

## Run & test
```bash
cd ragx
uv run pytest -q
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
