from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from thomas_ragx.platform.settings import Settings

TRACES_PATH = "/v1/traces"


def configure_tracing(settings: Settings) -> TracerProvider:
    resource = Resource.create(
        {"service.name": settings.service_name, "deployment.environment": settings.env}
    )
    provider = TracerProvider(resource=resource)
    if settings.otlp_endpoint:
        provider.add_span_processor(
            BatchSpanProcessor(OTLPSpanExporter(endpoint=settings.otlp_endpoint.rstrip("/") + TRACES_PATH))
        )
    return provider
