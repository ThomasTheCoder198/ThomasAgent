import httpx
import structlog
from minio import Minio
from opentelemetry.propagate import inject

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.tracing import configure_tracing

QDRANT_READY_PATH = "/readyz"
log = structlog.get_logger(__name__)


def ensure_buckets(config: Config) -> None:
    client = Minio(
        config.minio_endpoint,
        access_key=config.minio_access_key,
        secret_key=config.minio_secret_key.get_secret_value(),
        secure=config.minio_secure,
    )
    for bucket in config.buckets:
        if not client.bucket_exists(bucket):
            client.make_bucket(bucket)
            log.info("bucket created", bucket=bucket)


def check_qdrant(config: Config) -> None:
    headers: dict[str, str] = {}
    inject(headers)
    httpx.get(config.qdrant_url + QDRANT_READY_PATH, headers=headers).raise_for_status()


def run(config: Config) -> None:
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__)
    try:
        with tracer.start_as_current_span(
            "rag.bootstrap.run", record_exception=False, set_status_on_exception=False
        ):
            try:
                with tracer.start_as_current_span(
                    "rag.minio.ensure_buckets", record_exception=False, set_status_on_exception=False
                ):
                    ensure_buckets(config)
                    log.info("buckets ready")
                with tracer.start_as_current_span(
                    "rag.qdrant.check_ready", record_exception=False, set_status_on_exception=False
                ):
                    check_qdrant(config)
                    log.info("qdrant ready")
            except Exception as exc:
                log.error("bootstrap failed", error_type=type(exc).__name__)  # noqa: TRY400
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from exc
            log.info("bootstrap complete")
    finally:
        provider.shutdown()
