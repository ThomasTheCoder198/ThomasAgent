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
See the [RAG service conventions](../../README.md#conventions) for Config, credentials, shared naming, and boundary logging.

CLI bootstrap initializes logging with Config's service and `environment` before storage setup, so bucket creation and bootstrap lifecycle logs share the configured context. See [platform configuration aliases](platform/README.md#conventions) for preserved environment variable names. Logging follows the [shared field contract](../../../backend-core/internal/platform/logging/README.md#log-field-contract).

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
