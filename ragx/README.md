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
Config uses `RAG_` variables with non-secret defaults in `src/thomas_ragx/platform/config.yaml`. Required credentials are `RAG_CORE_SERVICE_TOKEN`, `RAG_MINIO_ACCESS_KEY`, and `RAG_MINIO_SECRET_KEY`. Process environment overrides `.env`, which overrides YAML. See the [glossary](../docs/glossary.md) for shared naming and response terms. Boundary logs omit exception text and document content.

Traces go to Phoenix on the ragx server (`deploy/ragx`). Logs stay on stdout as JSON with `request_id`, `trace_id`, and `span_id`; read them with `docker logs`. `RAG_PHOENIX_API_KEY` supplies the exporter's bearer credential and `RAG_PHOENIX_PROJECT` defaults to `thomas-ragx`. `RAG_TRACE_CONTENT` defaults to false; both CLI commands apply this policy before instrumentors start. The serve CLI installs the global tracer provider and flushes it after server shutdown.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
