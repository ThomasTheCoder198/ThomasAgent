# RAG unit tests

## Purpose
Checks config, errors, logging redaction, tracing, retries, API responses, and bootstrap behavior.

## Entry points
- `test_*.py`: pytest test modules.
- `test_config.py`: unchanged environment aliases and defaults for renamed Config fields, including construction by Python field name.
- `test_logging.py`: redaction and CLI bootstrap logging context.
- `test_health.py`: response format and HTTP handler/access log correlation.

## Dependencies
- Uses: pytest, service modules, and injected fakes.
- Used by: ragx test suite.

## Run & test
```bash
cd ragx
uv run pytest -q tests/unit
```

## Conventions
See the [RAG service conventions](../../README.md#conventions) for Config, credentials, shared naming, and boundary logging.

Logging regressions verify configured environment context, active spans, and generated or supplied request IDs in emitted JSON. Synchronous handler coverage checks context inheritance across the framework's thread boundary. The [shared field contract](../../../backend-core/internal/platform/logging/README.md#log-field-contract) is authoritative; existing redaction test bodies and assertions remain unchanged.

Config alias coverage tests the installed pydantic-settings resolution directly; see the [platform alias convention](../../src/thomas_ragx/platform/README.md#conventions).

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
