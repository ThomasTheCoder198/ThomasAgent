from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from thomas_ragx.platform.config import Config

TRACES_PATH = "/v1/traces"


def configure_tracing(config: Config) -> TracerProvider:
    resource = Resource.create(
        {"service.name": config.service_name, "deployment.environment": config.environment}
    )
    provider = TracerProvider(resource=resource)
    if config.otlp_endpoint:
        provider.add_span_processor(
            BatchSpanProcessor(OTLPSpanExporter(endpoint=config.otlp_endpoint.rstrip("/") + TRACES_PATH))
        )
    return provider
