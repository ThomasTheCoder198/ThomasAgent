# RAG service

## Purpose
Owns the Python RAG service foundations, API startup, and storage bootstrap.

## Entry points
- `ragx serve`: starts the API.
- `ragx bootstrap`: creates storage buckets.

## Dependencies
- Uses: FastAPI, Typer, uvicorn, structlog, OpenTelemetry, MinIO, and Qdrant.
- Used by: core service clients and deployment.

## Run & test
```bash
cd ragx
uv sync --frozen
uv run ragx serve
uv run ragx bootstrap
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
uv run mypy src
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
