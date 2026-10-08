import base64
import binascii
import json
import re
import uuid
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import datetime
from typing import Final
from uuid import UUID

from thomas_ragx.auth.caller import Caller
from thomas_ragx.db.outbox import (
    PURGE_KIND_FILE,
    STREAM_INGEST_REQUESTED,
    STREAM_KB_PURGE_REQUESTED,
    enqueue_outbox,
)
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import file_repository as files
from thomas_ragx.kb import repository as kbs
from thomas_ragx.kb.model import KbFile
from thomas_ragx.kb.repository import Connection
from thomas_ragx.kb.service import require_org
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage, preview_key, raw_key
from thomas_ragx.kb.upload import ReceivedFile
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

IDEMPOTENCY_KEY_PATTERN: Final = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
CURSOR_PARTS: Final = 2


def _field_error(field: str, code: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": code}]})


def encode_cursor(file: KbFile) -> str:
    raw = json.dumps([file.created_at.isoformat(), str(file.id)]).encode()
    return base64.urlsafe_b64encode(raw).decode()


def decode_cursor(value: str) -> tuple[datetime, UUID]:
    try:
        parts = json.loads(base64.urlsafe_b64decode(value.encode()))
        if not isinstance(parts, list) or len(parts) != CURSOR_PARTS:
            raise ValueError("cursor shape")  # noqa: TRY301 - reject malformed decoded data in this validator
        return datetime.fromisoformat(str(parts[0])), UUID(str(parts[1]))
    except (ValueError, binascii.Error, json.JSONDecodeError, TypeError):
        raise _field_error("cursor", "INVALID") from None


class FileService:
    def __init__(
        self,
        pool: Pool,
        storage: ObjectStorage,
        max_upload_bytes: int,
        multipart_overhead_bytes: int,
        idempotency_ttl_seconds: int,
        page_default_limit: int,
        page_max_limit: int,
    ) -> None:
        self._pool = pool
        self._storage = storage
        self.max_upload_bytes = max_upload_bytes
        self.multipart_overhead_bytes = multipart_overhead_bytes
        self._idempotency_ttl = idempotency_ttl_seconds
        self._page_default = page_default_limit
        self._page_max = page_max_limit

    @asynccontextmanager
    async def _transaction(self) -> AsyncIterator[Connection]:
        async with self._pool.connection() as conn, conn.transaction():
            yield conn

    async def _require_kb(self, conn: Connection, caller: Caller, kb_id: UUID) -> None:
        await require_org(conn, caller.org_id)
        if await kbs.get_kb(conn, caller.org_id, kb_id) is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)

    async def ensure_kb(self, caller: Caller, kb_id: UUID) -> None:
        """Checked before the body is read, so an upload to a missing KB never spools its file."""
        async with self._pool.connection() as conn:
            await self._require_kb(conn, caller, kb_id)

    async def _replay(self, conn: Connection, caller: Caller, kb_id: UUID, key: str | None) -> KbFile | None:
        if key is None:
            return None
        file_id = await files.remembered_file_id(conn, caller.org_id, key, self._idempotency_ttl)
        if file_id is None:
            return None
        file = await files.get_any_file(conn, caller.org_id, file_id)
        if file is None:
            # The file was deleted since: the key has nothing left to replay, so this upload starts fresh.
            await files.forget_file_id(conn, caller.org_id, key)
            return None
        if file.kb_id != kb_id:
            raise AppError(ErrorCode.CONFLICT)
        return file

    async def _existing(
        self, conn: Connection, caller: Caller, kb_id: UUID, received: ReceivedFile, key: str | None
    ) -> KbFile | None:
        replay = await self._replay(conn, caller, kb_id, key)
        if replay is not None:
            return replay
        duplicate = await files.find_by_sha(conn, kb_id, received.sha256)
        if (
            duplicate is not None
            and key is not None
            and not await files.remember_file_id(conn, caller.org_id, key, duplicate.id)
        ):
            raise AppError(ErrorCode.CONFLICT)
        return duplicate

    async def upload(
        self,
        caller: Caller,
        kb_id: UUID,
        received: ReceivedFile,
        idempotency_key: str | None,
        request_id: str,
    ) -> tuple[KbFile, bool]:
        if idempotency_key is not None and IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key) is None:
            raise _field_error("Idempotency-Key", "INVALID")
        async with self._transaction() as conn:
            await self._require_kb(conn, caller, kb_id)
            if await kbs.lock_kb(conn, caller.org_id, kb_id) is None:
                raise AppError(ErrorCode.KB_NOT_FOUND)
            existing = await self._existing(conn, caller, kb_id, received, idempotency_key)
        if existing is not None:
            return existing, False
        file_id = uuid.uuid4()
        key = raw_key(caller.org_id, kb_id, file_id)
        await self._storage.put_object(RAW_BUCKET, key, received.stream, received.size, received.content_type)
        try:
            stored, created = await self._insert(
                caller, kb_id, file_id, received, idempotency_key, request_id
            )
        except Exception:
            await self._storage.remove(RAW_BUCKET, key)
            raise
        if not created:
            await self._storage.remove(RAW_BUCKET, key)
        return stored, created

    async def _insert(
        self,
        caller: Caller,
        kb_id: UUID,
        file_id: UUID,
        received: ReceivedFile,
        idempotency_key: str | None,
        request_id: str,
    ) -> tuple[KbFile, bool]:
        async with self._transaction() as conn:
            # The KB row lock serializes uploads into one KB: versions and duplicate checks cannot race.
            if await kbs.lock_kb(conn, caller.org_id, kb_id) is None:
                raise AppError(ErrorCode.KB_NOT_FOUND)
            existing = await self._existing(conn, caller, kb_id, received, idempotency_key)
            if existing is not None:
                return existing, False
            version = await files.next_version(conn, kb_id, received.name)
            stored = await files.insert_file(conn, file_id, caller.org_id, kb_id, received, version)
            await enqueue_outbox(
                conn,
                caller.org_id,
                STREAM_INGEST_REQUESTED,
                {"kb_id": str(kb_id), "file_id": str(file_id), "version": version},
                request_id,
            )
            if idempotency_key is not None and not await files.remember_file_id(
                conn, caller.org_id, idempotency_key, file_id
            ):
                raise AppError(ErrorCode.CONFLICT)
            return stored, True

    async def list_page(
        self, caller: Caller, kb_id: UUID, cursor: str | None, limit: int | None
    ) -> tuple[list[KbFile], str | None, int]:
        size = self._page_default if limit is None else limit
        if size < 1 or size > self._page_max:
            raise _field_error("limit", "OUT_OF_RANGE")
        after = decode_cursor(cursor) if cursor else None
        async with self._pool.connection() as conn:
            await self._require_kb(conn, caller, kb_id)
            rows = await files.list_files(conn, caller.org_id, kb_id, after, size + 1)
        page = rows[:size]
        next_cursor = encode_cursor(page[-1]) if len(rows) > size else None
        return page, next_cursor, size

    async def delete(self, caller: Caller, kb_id: UUID, file_id: UUID, request_id: str) -> None:
        async with self._transaction() as conn:
            await self._require_kb(conn, caller, kb_id)
            if not await files.mark_file_deleted(conn, caller.org_id, kb_id, file_id):
                raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
            payload = {"kind": PURGE_KIND_FILE, "kb_id": str(kb_id), "file_id": str(file_id)}
            await enqueue_outbox(conn, caller.org_id, STREAM_KB_PURGE_REQUESTED, payload, request_id)

    async def retry(self, caller: Caller, kb_id: UUID, file_id: UUID, request_id: str) -> KbFile:
        async with self._transaction() as conn:
            await self._require_kb(conn, caller, kb_id)
            requeued = await files.requeue_failed(conn, caller.org_id, kb_id, file_id)
            if requeued is None:
                exists = await files.get_file(conn, caller.org_id, kb_id, file_id) is not None
                raise AppError(ErrorCode.CONFLICT if exists else ErrorCode.KB_FILE_NOT_FOUND)
            payload = {"kb_id": str(kb_id), "file_id": str(file_id), "version": requeued.version}
            await enqueue_outbox(conn, caller.org_id, STREAM_INGEST_REQUESTED, payload, request_id)
        return requeued

    async def _file(self, caller: Caller, kb_id: UUID, file_id: UUID) -> KbFile:
        async with self._pool.connection() as conn:
            await self._require_kb(conn, caller, kb_id)
            file = await files.get_file(conn, caller.org_id, kb_id, file_id)
        if file is None:
            raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
        return file

    async def open_raw(
        self, caller: Caller, kb_id: UUID, file_id: UUID
    ) -> tuple[KbFile, AsyncIterator[bytes]]:
        file = await self._file(caller, kb_id, file_id)
        return file, await self._storage.stream_object(RAW_BUCKET, raw_key(caller.org_id, kb_id, file_id))

    async def open_preview(self, caller: Caller, kb_id: UUID, file_id: UUID) -> AsyncIterator[bytes]:
        await self._file(caller, kb_id, file_id)
        return await self._storage.stream_object(PREVIEW_BUCKET, preview_key(caller.org_id, kb_id, file_id))
