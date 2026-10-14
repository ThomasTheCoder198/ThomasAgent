from typing import Final
from uuid import UUID

import structlog

from thomas_ragx.db.outbox import PURGE_KIND_FILE, PURGE_KIND_KB
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.kb import file_repository as files
from thomas_ragx.kb import repository as kbs
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage, kb_prefix, preview_key, raw_key
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.worker.consumer import StreamMessage

FIELD_KIND: Final = "kind"
FIELD_KB_ID: Final = "kb_id"
FIELD_FILE_ID: Final = "file_id"
log = structlog.get_logger(__name__)


def _uuid_field(message: StreamMessage, name: str) -> UUID:
    try:
        return UUID(str(message.payload[name]))
    except (KeyError, ValueError):
        raise AppError(ErrorCode.VALIDATION_FAILED) from None


def _catalog_code(code: str) -> str:
    # The DLQ also records exception type names; the file row only ever shows catalog codes.
    try:
        return ErrorCode(code).value
    except ValueError:
        return ErrorCode.INTERNAL_ERROR.value


class IngestDeadLetter:
    """After the last delivery the file is failed with a reason (spec Â§13), unless it already finished."""

    def __init__(self, pool: Pool) -> None:
        self._pool = pool

    async def __call__(self, message: StreamMessage, error_code: str) -> None:
        file_id = _uuid_field(message, FIELD_FILE_ID)
        code = _catalog_code(error_code)
        async with self._pool.connection() as conn:
            marked = await files.mark_ingest_failed(conn, message.org_id, file_id, code)
        if marked:
            log.info("ingest marked failed", file_id=str(file_id), error_code=code)
        else:
            log.info("ingest dead letter ignored, file finished", file_id=str(file_id), error_code=code)


class IngestHandler:
    def __init__(self, pipeline: IngestPipeline) -> None:
        self._pipeline = pipeline

    async def __call__(self, message: StreamMessage) -> None:
        await self._pipeline.run(IngestJob.from_message(message))


class PurgeHandler:
    def __init__(self, pool: Pool, storage: ObjectStorage, store: QdrantStore) -> None:
        self._pool = pool
        self._storage = storage
        self._store = store

    async def __call__(self, message: StreamMessage) -> None:
        kind = message.payload.get(FIELD_KIND)
        kb_id = _uuid_field(message, FIELD_KB_ID)
        if kind == PURGE_KIND_FILE:
            await self._purge_file(message.org_id, kb_id, _uuid_field(message, FIELD_FILE_ID))
        elif kind == PURGE_KIND_KB:
            await self._purge_kb(message.org_id, kb_id)
        else:
            raise AppError(ErrorCode.VALIDATION_FAILED)

    async def _purge_file(self, org_id: str, kb_id: UUID, file_id: UUID) -> None:
        async with self._pool.connection() as conn, conn.transaction():
            collection = await kbs.lock_collection(conn, org_id, kb_id)
            if collection is not None:
                await self._store.delete_file_points(collection, org_id, kb_id, file_id)
            await self._storage.remove(RAW_BUCKET, raw_key(org_id, kb_id, file_id))
            await self._storage.remove(PREVIEW_BUCKET, preview_key(org_id, kb_id, file_id))
            await files.delete_file_row(conn, org_id, file_id)
        log.info("file purged", kb_id=str(kb_id), file_id=str(file_id))

    async def _purge_kb(self, org_id: str, kb_id: UUID) -> None:
        async with self._pool.connection() as conn, conn.transaction():
            collection = await kbs.lock_collection(conn, org_id, kb_id)
            if collection is not None:
                await self._store.delete_kb_points(collection, org_id, kb_id)
            await self._storage.remove_prefix(RAW_BUCKET, kb_prefix(org_id, kb_id))
            await self._storage.remove_prefix(PREVIEW_BUCKET, kb_prefix(org_id, kb_id))
            await kbs.delete_kb_row(conn, org_id, kb_id)
        log.info("knowledge base purged", kb_id=str(kb_id))
