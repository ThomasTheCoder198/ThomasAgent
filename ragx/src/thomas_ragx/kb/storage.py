import asyncio
import hashlib
import io
from collections.abc import AsyncIterator
from contextlib import suppress
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Final
from uuid import UUID

from minio import Minio
from minio.deleteobjects import DeleteObject
from minio.error import S3Error

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

RAW_BUCKET: Final = "kb-raw"
PREVIEW_BUCKET: Final = "kb-preview"
STREAM_CHUNK_BYTES: Final = 1_048_576
MISSING_OBJECT_CODES: Final = frozenset({"NoSuchKey", "NoSuchObject", "NoSuchBucket"})
PREVIEW_SUFFIX: Final = ".json"


def kb_prefix(org_id: str, kb_id: UUID | str) -> str:
    return f"{org_id}/{kb_id}/"


def raw_key(org_id: str, kb_id: UUID | str, file_id: UUID | str) -> str:
    return f"{kb_prefix(org_id, kb_id)}{file_id}"


def preview_key(org_id: str, kb_id: UUID | str, file_id: UUID | str) -> str:
    return f"{kb_prefix(org_id, kb_id)}{file_id}{PREVIEW_SUFFIX}"


@dataclass(frozen=True)
class StoredObject:
    key: str
    last_modified: datetime


class ObjectStorage:
    """MinIO's SDK is synchronous; every call runs in a worker thread so the event loop never blocks."""

    def __init__(self, client: Minio) -> None:
        self._client = client

    async def put_bytes(self, bucket: str, key: str, data: bytes, content_type: str) -> None:
        await asyncio.to_thread(
            self._client.put_object, bucket, key, io.BytesIO(data), len(data), content_type=content_type
        )

    async def ready(self) -> bool:
        return await asyncio.to_thread(self._client.bucket_exists, RAW_BUCKET)

    async def size_of(self, bucket: str, key: str) -> int | None:
        """None when the object does not exist (yet)."""
        try:
            stat = await asyncio.to_thread(self._client.stat_object, bucket, key)
        except S3Error as exc:
            if exc.code in MISSING_OBJECT_CODES:
                return None
            raise
        return int(stat.size or 0)

    def _sha256_of(self, bucket: str, key: str) -> str | None:
        try:
            response = self._client.get_object(bucket, key)
        except S3Error as exc:
            if exc.code in MISSING_OBJECT_CODES:
                return None
            raise
        digest = hashlib.sha256()
        try:
            while chunk := response.read(STREAM_CHUNK_BYTES):
                digest.update(chunk)
        finally:
            response.close()
            response.release_conn()
        return digest.hexdigest()

    async def sha256_of(self, bucket: str, key: str) -> str | None:
        """Streams the object in chunks; None if it vanished meanwhile (a concurrent dedupe)."""
        return await asyncio.to_thread(self._sha256_of, bucket, key)

    def _download_to(self, bucket: str, key: str, path: Path) -> str | None:
        try:
            response = self._client.get_object(bucket, key)
        except S3Error as exc:
            if exc.code in MISSING_OBJECT_CODES:
                return None
            raise
        digest = hashlib.sha256()
        try:
            with path.open("wb") as sink:
                while chunk := response.read(STREAM_CHUNK_BYTES):
                    digest.update(chunk)
                    sink.write(chunk)
        except BaseException:
            # The writer has closed before unlink; cleanup must not hide the read/write failure.
            with suppress(OSError):
                path.unlink(missing_ok=True)
            raise
        finally:
            response.close()
            response.release_conn()
        return digest.hexdigest()

    async def download_to(self, bucket: str, key: str, path: Path) -> str | None:
        """One pass: the copy that gets parsed is exactly the bytes that were hashed (ruling A15)."""
        return await asyncio.to_thread(self._download_to, bucket, key, path)

    async def exists(self, bucket: str, key: str) -> bool:
        try:
            await asyncio.to_thread(self._client.stat_object, bucket, key)
        except S3Error as exc:
            if exc.code in MISSING_OBJECT_CODES:
                return False
            raise
        return True

    def _list_objects(self, bucket: str) -> list[StoredObject]:
        return [
            StoredObject(str(item.object_name), item.last_modified)
            for item in self._client.list_objects(bucket, recursive=True)
            if item.last_modified is not None
        ]

    async def list_objects(self, bucket: str) -> list[StoredObject]:
        return await asyncio.to_thread(self._list_objects, bucket)

    async def remove(self, bucket: str, key: str) -> None:
        await asyncio.to_thread(self._client.remove_object, bucket, key)

    def _remove_prefix(self, bucket: str, prefix: str) -> None:
        targets = [
            DeleteObject(str(item.object_name))
            for item in self._client.list_objects(bucket, prefix=prefix, recursive=True)
        ]
        # remove_objects is lazy: iterating it performs the deletes and yields only failures.
        for failure in self._client.remove_objects(bucket, targets):
            raise RuntimeError(f"object delete failed with {failure.code}")

    async def remove_prefix(self, bucket: str, prefix: str) -> None:
        await asyncio.to_thread(self._remove_prefix, bucket, prefix)

    async def stream_object(self, bucket: str, key: str) -> AsyncIterator[bytes]:
        """Opens the object first, so a missing object is a JSON 404 rather than a broken stream."""
        try:
            response = await asyncio.to_thread(self._client.get_object, bucket, key)
        except S3Error as exc:
            if exc.code in MISSING_OBJECT_CODES:
                raise AppError(ErrorCode.KB_FILE_NOT_FOUND) from None
            raise

        async def chunks() -> AsyncIterator[bytes]:
            try:
                while chunk := await asyncio.to_thread(response.read, STREAM_CHUNK_BYTES):
                    yield chunk
            finally:
                response.close()
                response.release_conn()

        return chunks()


def create_storage(config: Config) -> ObjectStorage:
    return ObjectStorage(
        Minio(
            config.minio_endpoint,
            access_key=config.minio_access_key,
            secret_key=config.minio_secret_key.get_secret_value(),
            secure=config.minio_secure,
        )
    )
