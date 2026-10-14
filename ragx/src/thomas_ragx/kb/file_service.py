import base64
import binascii
import json
import re
import uuid
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
from typing import Final
from uuid import UUID

import structlog

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
from thomas_ragx.kb.model import DownloadLink, KbFile, UploadSlot, UploadTicket
from thomas_ragx.kb.presign import UrlSigner
from thomas_ragx.kb.repository import Connection
from thomas_ragx.kb.service import require_org
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage, preview_key, raw_key
from thomas_ragx.kb.upload import content_type_for, field_error, normalize_file_name
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

IDEMPOTENCY_KEY_PATTERN: Final = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
CURSOR_PARTS: Final = 2
NAME_FIELD: Final = "name"
SIZE_FIELD: Final = "sizeBytes"
FILE_FIELD: Final = "file"
IDEMPOTENCY_KEY_FIELD: Final = "Idempotency-Key"
CODE_EMPTY: Final = "EMPTY"
CODE_MISSING: Final = "MISSING"
CODE_SIZE_MISMATCH: Final = "SIZE_MISMATCH"
CODE_INVALID: Final = "INVALID"
CODE_OUT_OF_RANGE: Final = "OUT_OF_RANGE"
log = structlog.get_logger(__name__)


def utc_now() -> datetime:
    return datetime.now(UTC)


def encode_cursor(file: KbFile) -> str:
    return encode_cursor_values(file.created_at, file.id)


def encode_cursor_values(created_at: datetime, file_id: UUID) -> str:
    raw = json.dumps([created_at.isoformat(), str(file_id)]).encode()
    return base64.urlsafe_b64encode(raw).decode()


def decode_cursor(value: str) -> tuple[datetime, UUID]:
    try:
        parts = json.loads(base64.urlsafe_b64decode(value.encode()))
        if not isinstance(parts, list) or len(parts) != CURSOR_PARTS:
            raise ValueError("cursor shape")  # noqa: TRY301 - reject malformed decoded data in this validator
        return datetime.fromisoformat(str(parts[0])), UUID(str(parts[1]))
    except (ValueError, binascii.Error, json.JSONDecodeError, TypeError):
        raise field_error("cursor", CODE_INVALID) from None


class FileService:
    def __init__(
        self,
        pool: Pool,
        storage: ObjectStorage,
        signer: UrlSigner,
        *,
        max_upload_bytes: int,
        upload_url_ttl: timedelta,
        download_url_ttl: timedelta,
        idempotency_ttl_seconds: int,
        page_default_limit: int,
        page_max_limit: int,
        now: Callable[[], datetime] = utc_now,
    ) -> None:
        self._pool = pool
        self._storage = storage
        self._signer = signer
        self.max_upload_bytes = max_upload_bytes
        self._upload_url_ttl = upload_url_ttl
        self._download_url_ttl = download_url_ttl
        self._idempotency_ttl = idempotency_ttl_seconds
        self._page_default = page_default_limit
        self._page_max = page_max_limit
        self._now = now

    @asynccontextmanager
    async def _transaction(self) -> AsyncIterator[Connection]:
        async with self._pool.connection() as conn, conn.transaction():
            yield conn

    async def _require_kb(self, conn: Connection, caller: Caller, kb_id: UUID) -> None:
        await require_org(conn, caller.org_id)
        if await kbs.get_kb(conn, caller.org_id, kb_id) is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)

    async def create_upload(
        self, caller: Caller, kb_id: UUID, raw_name: str, size_bytes: int
    ) -> UploadTicket:
        name = normalize_file_name(raw_name, NAME_FIELD)
        content_type = content_type_for(name)
        if size_bytes < 1:
            raise field_error(SIZE_FIELD, CODE_EMPTY)
        if size_bytes > self.max_upload_bytes:
            raise AppError(ErrorCode.PAYLOAD_TOO_LARGE)
        file_id = uuid.uuid4()
        async with self._transaction() as conn:
            await self._require_kb(conn, caller, kb_id)
            await files.insert_upload_slot(
                conn, file_id, caller.org_id, kb_id, name, content_type, size_bytes
            )
        expires_at = self._now() + self._upload_url_ttl
        post = self._signer.post_policy(
            RAW_BUCKET, raw_key(caller.org_id, kb_id, file_id), size_bytes, expires_at
        )
        return UploadTicket(file_id=file_id, post=post)

    async def confirm_upload(
        self, caller: Caller, kb_id: UUID, file_id: UUID, idempotency_key: str | None, request_id: str
    ) -> KbFile:
        """Idempotent by file_id (ruling P10): every repeat answers the same file."""
        if idempotency_key is not None and IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key) is None:
            raise field_error(IDEMPOTENCY_KEY_FIELD, CODE_INVALID)
        async with self._pool.connection() as conn:
            await self._require_kb(conn, caller, kb_id)
            settled = await self._settled(conn, caller, kb_id, file_id)
            if settled is not None:
                return settled
            slot = await files.get_upload_slot(conn, caller.org_id, kb_id, file_id)
        if slot is None:
            raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
        key = raw_key(caller.org_id, kb_id, file_id)
        size = await self._storage.size_of(RAW_BUCKET, key)
        if size is None:
            return await self._settled_or(caller, kb_id, file_id, CODE_MISSING)
        if size != slot.size_bytes:
            await self._storage.remove(RAW_BUCKET, key)
            raise field_error(FILE_FIELD, CODE_SIZE_MISMATCH)
        sha256 = await self._storage.sha256_of(RAW_BUCKET, key)
        if sha256 is None:
            return await self._settled_or(caller, kb_id, file_id, CODE_MISSING)
        stored, duplicate = await self._settle(caller, slot, sha256, request_id)
        if duplicate:
            await self._remove_orphan(key)
        return stored

    async def _remove_orphan(self, key: str) -> None:
        try:
            await self._storage.remove(RAW_BUCKET, key)
        except Exception as exc:  # the dedupe is committed; the sweeper removes the row-less object (A17)
            log.warning("duplicate object left for the sweep", error_type=type(exc).__name__)

    async def _settled(self, conn: Connection, caller: Caller, kb_id: UUID, file_id: UUID) -> KbFile | None:
        """The confirmed file, or the existing file a duplicate upload was folded into."""
        confirmed = await files.get_file(conn, caller.org_id, kb_id, file_id)
        if confirmed is not None:
            return confirmed
        folded_into = await files.remembered_file_id(
            conn, caller.org_id, str(file_id), self._idempotency_ttl, operation=files.OPERATION_CONFIRM
        )
        return None if folded_into is None else await files.get_file(conn, caller.org_id, kb_id, folded_into)

    async def _settled_or(self, caller: Caller, kb_id: UUID, file_id: UUID, code: str) -> KbFile:
        """A missing object is fine if a concurrent confirm already settled this upload."""
        async with self._pool.connection() as conn:
            settled = await self._settled(conn, caller, kb_id, file_id)
        if settled is None:
            raise field_error(FILE_FIELD, code)
        return settled

    async def _settle(
        self, caller: Caller, slot: UploadSlot, sha256: str, request_id: str
    ) -> tuple[KbFile, bool]:
        async with self._transaction() as conn:
            # The KB row lock serializes confirms into one KB: versions and the duplicate check cannot race.
            if await kbs.lock_kb(conn, caller.org_id, slot.kb_id) is None:
                raise AppError(ErrorCode.KB_NOT_FOUND)
            if await files.lock_upload_slot(conn, caller.org_id, slot.kb_id, slot.id) is None:
                settled = await self._settled(conn, caller, slot.kb_id, slot.id)  # a concurrent confirm won
                if settled is None:
                    raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
                return settled, False
            duplicate = await files.find_by_sha(conn, slot.kb_id, sha256)
            if duplicate is not None:
                await files.delete_upload_slot(conn, slot.id)
                await files.remember_file_id(
                    conn, caller.org_id, str(slot.id), duplicate.id, operation=files.OPERATION_CONFIRM
                )
                return duplicate, True
            version = await files.next_version(conn, slot.kb_id, slot.name)
            stored = await files.confirm_upload_slot(conn, slot.id, sha256, version)
            payload = {"kb_id": str(slot.kb_id), "file_id": str(slot.id), "version": version}
            await enqueue_outbox(conn, caller.org_id, STREAM_INGEST_REQUESTED, payload, request_id)
            return stored, False

    async def download_url(self, caller: Caller, kb_id: UUID, file_id: UUID) -> DownloadLink:
        file = await self._file(caller, kb_id, file_id)
        issued_at = self._now()
        url = self._signer.download_url(
            RAW_BUCKET, raw_key(caller.org_id, kb_id, file_id), file.name, self._download_url_ttl, issued_at
        )
        return DownloadLink(url=url, expires_at=issued_at + self._download_url_ttl)

    async def list_page(
        self, caller: Caller, kb_id: UUID, cursor: str | None, limit: int | None
    ) -> tuple[list[KbFile], str | None, int]:
        size = self._page_default if limit is None else limit
        if size < 1 or size > self._page_max:
            raise field_error("limit", CODE_OUT_OF_RANGE)
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

    async def open_preview(self, caller: Caller, kb_id: UUID, file_id: UUID) -> AsyncIterator[bytes]:
        await self._file(caller, kb_id, file_id)
        return await self._storage.stream_object(PREVIEW_BUCKET, preview_key(caller.org_id, kb_id, file_id))

    async def preview_size(self, caller: Caller, kb_id: UUID, file_id: UUID) -> int:
        await self._file(caller, kb_id, file_id)
        size = await self._storage.size_of(PREVIEW_BUCKET, preview_key(caller.org_id, kb_id, file_id))
        if size is None:
            raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
        return size
