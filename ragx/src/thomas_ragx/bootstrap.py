import asyncio

import httpx
import structlog
from minio import Minio
from opentelemetry.propagate import inject

from thomas_ragx.admin.service import ensure_admin_account
from thomas_ragx.db.migrate import migrate_up
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.tracing import configure_tracing
from thomas_ragx.resources import Resources
from thomas_ragx.retrieval.qdrant_store import QdrantStore

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


async def ensure_kb_collections(config: Config) -> int:
    """Bootstrap prepares live KB collections; later KBs are ensured at first index (spec section 18.8)."""
    resources = Resources.from_config(config)
    await resources.open()
    try:
        async with resources.pool.connection() as conn:
            rows = await (
                await conn.execute(
                    b"SELECT DISTINCT collection_name, embedding_dims "
                    b"FROM knowledge_bases WHERE deleted_at IS NULL"
                )
            ).fetchall()
        store = QdrantStore(resources.qdrant, upsert_batch_size=config.qdrant_upsert_batch_size)
        for row in rows:
            await store.ensure_collection(row["collection_name"], row["embedding_dims"])
        return len(rows)
    finally:
        await resources.close()


def run(config: Config) -> None:
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__)
    try:
        with tracer.start_as_current_span(
            "rag.bootstrap.run", record_exception=False, set_status_on_exception=False
        ):
            try:
                with tracer.start_as_current_span(
                    "rag.db.migrate", record_exception=False, set_status_on_exception=False
                ):
                    applied = migrate_up(config.database_url.get_secret_value())
                    log.info("migrations ready", applied=applied)
                with tracer.start_as_current_span(
                    "rag.admin.ensure_account", record_exception=False, set_status_on_exception=False
                ):
                    asyncio.run(ensure_admin_account(config))
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
                with tracer.start_as_current_span(
                    "rag.qdrant.ensure_collections", record_exception=False, set_status_on_exception=False
                ):
                    count = asyncio.run(ensure_kb_collections(config))
                    log.info("collections ready", count=count)
            except Exception as exc:
                log.error("bootstrap failed", error_type=type(exc).__name__)  # noqa: TRY400
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from exc
            log.info("bootstrap complete")
    finally:
        provider.shutdown()
