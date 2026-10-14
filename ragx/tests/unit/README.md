# RAG unit tests

## Purpose
Checks config, errors, logging redaction, tracing, retries, API responses, and bootstrap behavior.

## Entry points
- `test_*.py`: pytest test modules.
- `test_config.py`: unchanged environment aliases and defaults for renamed Config fields, including construction by Python field name.
- `test_logging.py`: redaction and CLI bootstrap logging context.
- `test_health.py`: response format and HTTP handler/access log correlation.
- `test_readiness.py`: dependency readiness and health separation using fake resources.
- `test_tracing.py`: Phoenix bearer export and project resource, malformed-key rejection without credential leakage in standard-library logs, content policy, isolated app providers, direct root-span/request-ID correlation, error-response trace headers, and exporter outage tolerance. CLI wiring checks content policy and global installation before startup plus provider shutdown on both normal and failed startup.
- `test_eval_report.py`: percentile, text normalization and golden-set loading for the eval harness.
- `test_cli_eval.py`: `ragx eval run` exit codes, `error_code` logging, provider shutdown and completion log fields.
- `test_search_factory.py`: `create_search_service` hands the metrics recorder to the embedder and reranker.
- `test_worker_metrics.py`: worker job counter and duration export to stdout with bounded labels.

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

Segmenter tests verify Vietnamese compounds and factory selection. Sparse encoder tests block network access and verify matching query/passage tokens. Readiness tests include Qdrant failure.
