from collections.abc import MutableMapping
from typing import Final

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

TRACES_PATH: Final = "/v1/traces"
RAGX_TRACE_ID_HEADER: Final = "X-Ragx-Trace-Id"
ATTR_REQUEST_ID: Final = "request_id"
ATTR_SPAN_KIND: Final = "openinference.span.kind"
SPAN_KIND_CHAIN: Final = "CHAIN"
ATTR_PROJECT_NAME: Final = "openinference.project.name"
AUTHORIZATION_HEADER: Final = "authorization"
BEARER_PREFIX: Final = "Bearer "
HIDE_INPUTS_ENV: Final = "OPENINFERENCE_HIDE_INPUTS"
HIDE_OUTPUTS_ENV: Final = "OPENINFERENCE_HIDE_OUTPUTS"
TRACE_ID_FORMAT: Final = "032x"


def _exporter_headers(config: Config) -> dict[str, str]:
    if config.phoenix_api_key is None:
        return {}
    key = config.phoenix_api_key.get_secret_value()
    if not key:
        return {}
    # HTTP client errors can echo malformed header values through the exporter's stdlib logger.
    if not key.isascii() or not key.isprintable() or any(character.isspace() for character in key):
        raise AppError(ErrorCode.VALIDATION_FAILED)
    return {AUTHORIZATION_HEADER: BEARER_PREFIX + key}


def configure_tracing(config: Config) -> TracerProvider:
    headers = _exporter_headers(config) if config.otlp_endpoint else {}
    resource = Resource.create(
        {
            "service.name": config.service_name,
            "deployment.environment": config.environment,
            ATTR_PROJECT_NAME: config.phoenix_project,
        }
    )
    provider = TracerProvider(resource=resource)
    if config.otlp_endpoint:
        exporter = OTLPSpanExporter(endpoint=config.otlp_endpoint.rstrip("/") + TRACES_PATH, headers=headers)
        provider.add_span_processor(BatchSpanProcessor(exporter))
    return provider


def install_global_tracer_provider(provider: TracerProvider) -> None:
    """Only the process entry point installs the one global OpenTelemetry provider."""
    trace.set_tracer_provider(provider)


def apply_trace_content_policy(config: Config, environ: MutableMapping[str, str]) -> None:
    """Instrumentors read these variables, keeping content hidden unless explicitly enabled."""
    hidden = "false" if config.trace_content else "true"
    environ[HIDE_INPUTS_ENV] = hidden
    environ[HIDE_OUTPUTS_ENV] = hidden


def format_trace_id(trace_id: int) -> str:
    return format(trace_id, TRACE_ID_FORMAT)
