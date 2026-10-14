# RAG service

## Purpose
Owns the Python RAG service foundations, API startup, and storage bootstrap.

## Entry points
- `ragx serve`: starts the API.
- `ragx bootstrap`: creates storage buckets.
- `ragx migrate up|down`: applies or reverts database migrations.
- `ragx eval run --set PATH --org ORG --kb KB_ID`: scores retrieval on a golden set ([eval harness](src/thomas_ragx/eval/README.md); `task ragx:eval` runs it in the worker container).

## Dependencies
- Uses: FastAPI, Typer, uvicorn, structlog, OpenTelemetry, Postgres, Redis, MinIO, and Qdrant.
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
Config uses `RAG_` variables with non-secret defaults in `src/thomas_ragx/platform/config.yaml`. Required secrets: `RAG_DATABASE_URL`, `RAG_VAULT_MASTER_KEY`, `RAG_CORE_PUBLIC_KEYS`, `RAG_MINIO_ACCESS_KEY` and `RAG_MINIO_SECRET_KEY`. Ragx never calls core. Process environment overrides `.env`, which overrides YAML. See the [glossary](../docs/glossary.md) for shared naming and response terms. Boundary logs omit exception text and document content.

Traces go to Phoenix on the ragx server (`deploy/ragx`). Logs stay on stdout as JSON with `request_id`, `trace_id`, and `span_id`; read them with `docker logs`. `RAG_PHOENIX_API_KEY` supplies the exporter's bearer credential and `RAG_PHOENIX_PROJECT` defaults to `thomas-ragx`. `RAG_TRACE_CONTENT` defaults to false; both CLI commands apply this policy before instrumentors start. The serve CLI installs the global tracer provider and flushes it after server shutdown.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.

Qdrant is required at API and worker runtime. Indexed readiness includes dense/BM25 vectors and preview JSON. The image sets HF_HUB_OFFLINE=1; BM25 uses a local directory and requires no model download at startup. The offline development embedding seed intentionally fails at the provider boundary and can be replayed from the DLQ.
