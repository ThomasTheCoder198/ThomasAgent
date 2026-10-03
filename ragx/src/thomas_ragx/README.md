# RAG package

## Purpose
Coordinates API startup and idempotent storage bootstrap.

## Entry points
- `cli.py`: serve and bootstrap commands.
- `bootstrap.py`: storage setup.

## Dependencies
- Uses: api, platform, MinIO, and Qdrant.
- Used by: installed ragx CLI and tests.

## Run & test
```bash
cd ragx
uv run ragx --help
uv run pytest -q
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
