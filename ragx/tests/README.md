# RAG tests

## Purpose
Owns regression coverage for the Python service foundations.

## Entry points
- `unit/`: platform, API, and bootstrap regression tests.
- `integration/`: Postgres, Redis, and MinIO testcontainers coverage.

## Dependencies
- Uses: pytest, service code, and test fakes.
- Used by: task test:ragx and CI.

## Run & test
```bash
cd ragx
uv run pytest -q
```

## Conventions
See the [RAG service conventions](../README.md#conventions) for Config, credentials, shared naming, and boundary logging.

Unit regressions exercise Config environment aliases and defaults, JSON logging through CLI bootstrap with fake storage/network ports, and real HTTP middleware and handlers. See [unit coverage](unit/README.md) and the [shared log field contract](../../backend-core/internal/platform/logging/README.md#log-field-contract).

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.

Retrieval coverage includes offline Vietnamese BM25 unit tests and real Qdrant tenant isolation, payload and replacement integration tests.

M1.3 KB tools are covered by `integration/test_tools_api.py` using indexed synthetic data and fake providers, `unit/test_manifest.py` for closed schemas, and `unit/test_tool_service.py` for section bounds and safe deadline spans. The common fixture closes each API metrics provider even for clients that omit ASGI lifespan, keeping periodic exports inside test capture lifetime.
