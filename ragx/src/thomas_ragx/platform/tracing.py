from collections.abc import MutableMapping
from typing import Any, Final

from openinference.instrumentation import TraceConfig
from opentelemetry import trace
from opentelemetry.context import Context
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import Span, SpanProcessor, TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import Status, StatusCode
from opentelemetry.util.types import Attributes

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

TRACES_PATH: Final = "/v1/traces"
RAGX_TRACE_ID_HEADER: Final = "X-Ragx-Trace-Id"
ATTR_REQUEST_ID: Final = "request_id"
ATTR_ORG_ID: Final = "org_id"
ATTR_TOOL: Final = "tool"
ATTR_SPAN_KIND: Final = "openinference.span.kind"
SPAN_KIND_CHAIN: Final = "CHAIN"
ATTR_PROJECT_NAME: Final = "openinference.project.name"
AUTHORIZATION_HEADER: Final = "authorization"
BEARER_PREFIX: Final = "Bearer "
HIDE_INPUTS_ENV: Final = "OPENINFERENCE_HIDE_INPUTS"
HIDE_OUTPUTS_ENV: Final = "OPENINFERENCE_HIDE_OUTPUTS"
TRACE_ID_FORMAT: Final = "032x"
ATTR_INPUT_VALUE: Final = "input.value"
ATTR_OUTPUT_VALUE: Final = "output.value"
EMBEDDING_TEXT_SUFFIX: Final = ".embedding.text"
EMBEDDING_VECTOR_SUFFIX: Final = ".embedding.vector"
SPAN_RECORD_EXCEPTION_METHOD: Final = "record_exception"
SPAN_SET_STATUS_METHOD: Final = "set_status"


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


class _ContentTraceConfig(TraceConfig):
    def mask(
        self,
        key: str,
        value: Any,
        *,
        externalize: bool = True,
    ) -> Any:
        # The upstream input policy leaves embedding text visible and retains redacted input.value.
        if self.hide_inputs and (key == ATTR_INPUT_VALUE or key.endswith(EMBEDDING_TEXT_SUFFIX)):
            return None
        if self.hide_outputs and (key == ATTR_OUTPUT_VALUE or key.endswith(EMBEDDING_VECTOR_SUFFIX)):
            return None
        return super().mask(key, value, externalize=externalize)


class _ProviderExceptionPolicy(SpanProcessor):
    def on_start(self, span: Span, parent_context: Context | None = None) -> None:
        record = span.record_exception
        set_status = span.set_status

        def record_exception(
            exception: BaseException,
            attributes: Attributes = None,
            timestamp: int | None = None,
            escaped: bool = False,
        ) -> None:
            # Provider errors can echo keys and input even when trace content is enabled.
            exception_type = type(exception).__name__
            record(
                Exception(exception_type),
                attributes={"exception.type": exception_type},
                timestamp=timestamp,
                escaped=escaped,
            )

        def safe_status(status: Status | StatusCode, description: str | None = None) -> None:
            set_status(Status(status.status_code if isinstance(status, Status) else status))

        # Intercept public recording methods before any instrumentor can attach unsafe events/status.
        setattr(span, SPAN_RECORD_EXCEPTION_METHOD, record_exception)
        setattr(span, SPAN_SET_STATUS_METHOD, safe_status)


def instrument_litellm(provider: TracerProvider) -> None:
    """Call after apply_trace_content_policy: the instrumentor reads OPENINFERENCE_HIDE_* when it starts."""
    from openinference.instrumentation.litellm import LiteLLMInstrumentor

    provider.add_span_processor(_ProviderExceptionPolicy())
    LiteLLMInstrumentor().instrument(tracer_provider=provider, config=_ContentTraceConfig())
