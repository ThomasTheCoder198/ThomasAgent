from typing import Final
from uuid import UUID

import structlog

from thomas_ragx.db.outbox import PURGE_KIND_FILE, PURGE_KIND_KB
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import file_repository as files
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage, kb_prefix, preview_key, raw_key
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
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


class IngestHandler:
    """M1.1 proves outbox → stream → consumer end to end; M1.3 replaces it with parse → index."""

    def __init__(self, pool: Pool, storage: ObjectStorage) -> None:
        self._pool = pool
        self._storage = storage

    async def __call__(self, message: StreamMessage) -> None:
        kb_id, file_id = _uuid_field(message, FIELD_KB_ID), _uuid_field(message, FIELD_FILE_ID)
        async with self._pool.connection() as conn:
            file = await files.get_file(conn, message.org_id, kb_id, file_id)
        if file is None:
            log.info("ingest skipped, file deleted", kb_id=str(kb_id), file_id=str(file_id))
            return
        if not await self._storage.exists(RAW_BUCKET, raw_key(message.org_id, kb_id, file_id)):
            raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
        log.info("ingest received", kb_id=str(kb_id), file_id=str(file_id), version=file.version)


class PurgeHandler:
    def __init__(self, pool: Pool, storage: ObjectStorage) -> None:
        self._pool = pool
        self._storage = storage

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
        await self._storage.remove(RAW_BUCKET, raw_key(org_id, kb_id, file_id))
        await self._storage.remove(PREVIEW_BUCKET, preview_key(org_id, kb_id, file_id))
        async with self._pool.connection() as conn:
            await files.delete_file_row(conn, org_id, file_id)
        log.info("file purged", kb_id=str(kb_id), file_id=str(file_id))

    async def _purge_kb(self, org_id: str, kb_id: UUID) -> None:
        # Qdrant points are deleted here too once M1.3 indexes them.
        await self._storage.remove_prefix(RAW_BUCKET, kb_prefix(org_id, kb_id))
        await self._storage.remove_prefix(PREVIEW_BUCKET, kb_prefix(org_id, kb_id))
        async with self._pool.connection() as conn:
            await conn.execute(
                b"DELETE FROM knowledge_bases WHERE id = %s AND org_id = %s AND deleted_at IS NOT NULL",
                (kb_id, org_id),
            )
        log.info("knowledge base purged", kb_id=str(kb_id))
