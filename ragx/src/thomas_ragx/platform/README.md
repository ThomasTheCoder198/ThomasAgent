# RAG platform

## Purpose
Owns typed config, catalog errors, safe structured logging, tracing, and retry helpers.

## Entry points
- `config.py`, `config.yaml`: typed configuration and non-secret defaults.
- `constants.py`: shared HTTP status thresholds used by responses and retries.
- `errors.py`: AppError and language selection.
- `error_codes_gen.py`: shared definitions covering authentication, internal service-token validation, and provider/model registry errors.
- `http_response.py`: success_body, error_response, request_id_of, and exception handlers; error bodies carry `requestId`.
- `request_id.py`: shared request id header, pattern and prefixes (mirrored from `contracts/request-id.json`, checked by `test_request_id.py`), `is_valid_request_id`, and `accept_request_id`.
- `request_state.py`: request.state names shared by auth and the access log.
- `logging.py`, `retry.py`: shared logging and retry infrastructure.
- `tracing.py`: tracer provider exporting to Phoenix (`RAG_OTLP_ENDPOINT` + `/v1/traces`, bearer `RAG_PHOENIX_API_KEY`, project `RAG_PHOENIX_PROJECT`), `install_global_tracer_provider` (CLI only), and `apply_trace_content_policy` (`RAG_TRACE_CONTENT`, default off).

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
See the [RAG service conventions](../../../README.md#conventions) for Config, credentials, shared naming, and boundary logging. Generated error_codes_gen.py is owned by task gen.

Config uses `environment`, `retry_base_delay_seconds`, and `retry_max_delay_seconds` in Python. Explicit validation aliases preserve `RAG_ENV`, `RAG_RETRY_BASE_DELAY_S`, and `RAG_RETRY_MAX_DELAY_S`; aliases contain the complete variable name because pydantic-settings does not apply `env_prefix` to them. Name validation also allows callers to construct Config with these Python field names, with constructor values taking precedence over environment values. Non-secret defaults live in `config.yaml`.

Presigned URLs use four settings: `RAG_S3_PUBLIC_ENDPOINT` (the origin browsers reach MinIO at, a full `http(s)://host[:port]` URL with no path; dev `http://localhost:9000`), `RAG_S3_REGION` (default `us-east-1`), `RAG_UPLOAD_URL_TTL` (YAML `900` seconds) and `RAG_DOWNLOAD_URL_TTL` (YAML `60`). TTLs are `timedelta` fields between 1 second and 7 days: YAML holds whole seconds, and environment overrides are ISO 8601 durations (`PT15M`, `PT60S`); a bare `"30"` is rejected.

Configuration precedence is constructor > process environment > `.env` > `config.yaml`. `load_config` can read a different dotenv path through `RAG_ENV_FILE`. Compose passes `deploy/ragx/.env` through `env_file` and derives only network endpoints and the MinIO/Phoenix credentials shared with other containers. The environment and dotenv sources disable field-name validation so renamed Python attributes cannot create unplanned environment spellings.

RAG pipeline settings (M1.3, defaults in `config.yaml`):
- `RAG_UPLOAD_MAX_BYTES` is 52428800 (50 MB since 2026-10-11); `contracts/kb-upload.json` `maxFileBytes` must equal it.
- `RAG_INGEST_CONCURRENCY` (2, at least 1) is the number of sequential ingest consumers per worker.
- `RAG_INGEST_VISIBILITY_TIMEOUT_MILLISECONDS` (1800000) is the ingest stream's reclaim time and the file lease length. `Config` rejects a value at or below `2 x RAG_DATALAB_TIMEOUT_SECONDS x 1000 + RAG_STREAM_VISIBILITY_TIMEOUT_MILLISECONDS`: a healthy slow convert (and its quality retry) must never be reclaimed and run twice (ruling A11).
- `RAG_WORKER_HEARTBEAT_INTERVAL_SECONDS` (10.0), `RAG_SWEEP_INTERVAL_SECONDS` (600.0), `RAG_UPLOAD_SWEEP_GRACE_SECONDS` (300, added to `RAG_UPLOAD_URL_TTL` for the sweep cutoff) and `RAG_WORK_DIR` (`/tmp/ragx-work`, temp copies of raw files).
- Datalab: `RAG_DATALAB_API_KEY` is optional and secret (`.env` only); without it ingest parses locally and marks files `degraded`. `RAG_OCR_BASE_URL`, `RAG_DATALAB_MODE` (`balanced`), `RAG_DATALAB_RETRY_MODE` (`accurate`), `RAG_DATALAB_MIN_QUALITY` (3.0), `RAG_DATALAB_TIMEOUT_SECONDS` (600.0), `RAG_DATALAB_POLL_INTERVAL_SECONDS` (2) and `RAG_DATALAB_SKIP_CACHE` (true). Ragx never reads the SDK's own `DATALAB_*` variables.

`HTTP_STATUS_CLIENT_ERROR`, `HTTP_STATUS_RATE_LIMITED` and `HTTP_STATUS_SERVER_ERROR` in `constants.py` provide the shared logging, retry and HTTP response thresholds.

Logging recursively copies and redacts dictionaries, lists, and tuples, including secret ancestor keys and bearer/sk values. Usage fields such as input_tokens remain visible, and input containers are not mutated.

`configure_logging(level, service, environment)` binds the configured service and environment once in contextvars. Request middleware adds request context, and active spans supply correlation. See the shared [log field contract](../../../../backend-core/internal/platform/logging/README.md#log-field-contract).

Each app owns its tracer provider; the factory never installs a global provider. The serve CLI installs it for future OpenInference instrumentors and shuts it down after uvicorn exits, flushing pending exports. Both CLI commands set `OPENINFERENCE_HIDE_INPUTS` and `OPENINFERENCE_HIDE_OUTPUTS` from `RAG_TRACE_CONTENT`, overriding stale environment values so prompts and responses remain hidden by default. Phoenix credentials use `SecretStr` and are sent only in the exporter's authorization header.

When tracing is configured, Phoenix keys containing whitespace, control characters, or non-ASCII text fail with the catalog `VALIDATION_FAILED` error before a provider or exporter is built. This prevents HTTP header validation exceptions from leaking credentials through the exporter's standard-library logger. Missing, empty, or ordinary incorrect keys still allow startup; Phoenix rejects their exports while the API keeps serving.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
