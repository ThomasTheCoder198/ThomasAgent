# RAG package

## Purpose
Coordinates API startup and idempotent storage bootstrap.

## Entry points
- `cli.py`: serve and bootstrap commands.
- `bootstrap.py`: migrations, the one-time admin account (`admin/`), buckets and the Qdrant check.
- `ui/`: the `/ui` admin pages for providers, models and roles.
- `parsing/`: block model, locators, parser registry and local parsers ([README](parsing/README.md)).
- `resources.py`: Postgres pool, Redis client and MinIO storage lifecycle and readiness.

## Dependencies
- Uses: api, admin, ui, platform, db, kb storage, Postgres, Redis, MinIO, and Qdrant.
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

- `parsing/`: local parsers and Datalab policy normalize blocks and locators.
- `ingest/`: claims uploads, verifies hashes, builds outlines and persists blocks and tables.

## Provider tracing startup
The `serve` and worker process entry points install LiteLLM OpenInference instrumentation after applying the trace content policy and installing the process tracer provider. Hidden content omits embedding text/vectors as well as generic input/output values.

`Resources` owns and closes Qdrant alongside Postgres and Redis. Readiness probes `get_collections`; bootstrap prepares dense/BM25 collections for every live knowledge base.
