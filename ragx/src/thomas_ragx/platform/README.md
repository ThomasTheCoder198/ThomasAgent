# RAG platform

## Purpose
Owns typed config, catalog errors, safe structured logging, tracing, and retry helpers.

## Entry points
- `config.py`: Config and load_config.
- `constants.py`: shared HTTP status thresholds used by responses and retries.
- `errors.py`: AppError and language selection.
- `error_codes_gen.py`: shared definitions covering authentication, internal service-token validation, and provider/model registry errors.
- `http_response.py`: success_body, error_response, request_id_of, and exception handlers; error bodies carry `requestId`.
- `request_id.py`: shared request id header, pattern and prefixes (mirrored from `contracts/request-id.json`, checked by `test_request_id.py`), `is_valid_request_id`, and `accept_request_id`.
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

Config uses `environment`, `retry_base_delay_seconds`, and `retry_max_delay_seconds` in Python. Explicit validation aliases preserve `RAG_ENV`, `RAG_RETRY_BASE_DELAY_S`, and `RAG_RETRY_MAX_DELAY_S`; aliases contain the complete variable name because pydantic-settings does not apply `env_prefix` to them. Name validation also allows callers to construct Config with these Python field names, with constructor values taking precedence over environment values. Defaults remain `dev`, `0.2` seconds, and `10.0` seconds.

The environment source receives a private config copy with field-name validation disabled. This keeps renamed constructor fields from implicitly enabling new environment variable spellings; the model and constructor source retain name validation. Source ordering and the disabled dotenv default are unchanged.

`HTTP_STATUS_RATE_LIMITED` and `HTTP_STATUS_SERVER_ERROR` in `constants.py` provide the shared retry and HTTP response thresholds.

Logging recursively copies and redacts dictionaries, lists, and tuples, including secret ancestor keys and bearer/sk values. Usage fields such as input_tokens remain visible, and input containers are not mutated.

`configure_logging(level, service, environment)` binds the configured service and environment once in contextvars. Request middleware adds request context, and active spans supply correlation. See the shared [log field contract](../../../../backend-core/internal/platform/logging/README.md#log-field-contract).

Each app owns its tracer provider; the factory never installs a global provider. The serve CLI installs it for future OpenInference instrumentors and shuts it down after uvicorn exits, flushing pending exports. Both CLI commands set `OPENINFERENCE_HIDE_INPUTS` and `OPENINFERENCE_HIDE_OUTPUTS` from `RAG_TRACE_CONTENT`, overriding stale environment values so prompts and responses remain hidden by default. Phoenix credentials use `SecretStr` and are sent only in the exporter's authorization header.

When tracing is configured, Phoenix keys containing whitespace, control characters, or non-ASCII text fail with the catalog `VALIDATION_FAILED` error before a provider or exporter is built. This prevents HTTP header validation exceptions from leaking credentials through the exporter's standard-library logger. Missing, empty, or ordinary incorrect keys still allow startup; Phoenix rejects their exports while the API keeps serving.

## Common failures
- Missing credentials: supply the required RAG_ environment variables.
- Bootstrap connection failure: start MinIO and Qdrant.
