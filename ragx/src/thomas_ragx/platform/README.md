# RAG platform

## Purpose
Owns typed settings, catalog errors, safe structured logging, tracing, and retry helpers.

## Entry points
- `settings.py`: Settings and get_settings.
- `errors.py`: AppError and exception handlers.
- `logging.py`, `tracing.py`, `retry.py`: shared infrastructure.

## Dependencies
- Uses: generated catalog, pydantic-settings, structlog, and OpenTelemetry.
- Used by: API, CLI, bootstrap, and tests.

## Run & test
```bash
task gen
cd ragx
uv run mypy src
uv run pytest -q tests/unit
```

## Conventions
Settings use RAG_ variables. Required credentials are RAG_CORE_SERVICE_TOKEN, RAG_MINIO_ACCESS_KEY, and RAG_MINIO_SECRET_KEY. Errors use catalog codes and envelopes; boundary logs omit exception text and document content. Generated error_codes_gen.py is owned by task gen.

Logging recursively copies and redacts dictionaries, lists, and tuples, including secret ancestor keys and bearer/sk values. Usage fields such as input_tokens remain visible, and input containers are not mutated.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
