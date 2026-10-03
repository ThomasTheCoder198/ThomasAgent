# RAG unit tests

## Purpose
Checks settings, errors, logging redaction, tracing, retries, API responses, and bootstrap behavior.

## Entry points
- `test_*.py`: pytest test modules.

## Dependencies
- Uses: pytest, service modules, and injected fakes.
- Used by: ragx test suite.

## Run & test
```bash
cd ragx
uv run pytest -q tests/unit
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
