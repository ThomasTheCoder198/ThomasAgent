import asyncio
import hashlib
import unicodedata
from collections.abc import AsyncIterator, Mapping
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import BinaryIO, Final

from starlette.datastructures import UploadFile
from starlette.requests import Request
from starlette.types import Message

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

FILE_FIELD: Final = "file"
FORM_MAX_FILES: Final = 1
FORM_MAX_FIELDS: Final = 1
MAX_NAME_LENGTH: Final = 255
HASH_CHUNK_BYTES: Final = 1_048_576
CONTENT_LENGTH_HEADER: Final = "content-length"
RESERVED_NAMES: Final = frozenset({".", ".."})
# Main spec §4.1; the extension decides the type because a client's Content-Type is not trusted.
SUPPORTED_EXTENSIONS: Final[Mapping[str, str]] = {
    ".pdf": "application/pdf",
    ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ".csv": "text/csv",
    ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
    ".md": "text/markdown",
    ".txt": "text/plain",
    ".html": "text/html",
    ".htm": "text/html",
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
}


@dataclass(frozen=True)
class ReceivedFile:
    name: str
    content_type: str
    size: int
    sha256: str
    stream: BinaryIO


def _field_error(field: str, code: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": code}]})


def normalize_file_name(raw: str) -> str:
    # NFC so a macOS (NFD) upload and a Windows (NFC) upload of the same name are one file with two versions.
    normalized = unicodedata.normalize("NFC", raw)
    if any(unicodedata.category(char) == "Cc" for char in normalized):
        raise _field_error(FILE_FIELD, "INVALID_NAME")
    name = normalized.replace("\\", "/").rsplit("/", 1)[-1].strip()
    if not name or name in RESERVED_NAMES or len(name) > MAX_NAME_LENGTH:
        raise _field_error(FILE_FIELD, "INVALID_NAME")
    return name


def content_type_for(name: str) -> str:
    content_type = SUPPORTED_EXTENSIONS.get(PurePosixPath(name).suffix.lower())
    if content_type is None:
        raise AppError(ErrorCode.UNSUPPORTED_MEDIA_TYPE)
    return content_type


def check_declared_length(value: str | None, max_bytes: int, overhead_bytes: int) -> None:
    """Runs before the body is read, so the multipart parser's temp file is bounded by Content-Length."""
    if value is None:
        raise _field_error("Content-Length", "REQUIRED")
    try:
        declared = int(value)
    except ValueError:
        raise _field_error("Content-Length", "INVALID") from None
    if declared < 0:
        raise _field_error("Content-Length", "INVALID")
    if declared > max_bytes + overhead_bytes:
        raise AppError(ErrorCode.PAYLOAD_TOO_LARGE)


def _measure(stream: BinaryIO) -> tuple[int, str]:
    digest = hashlib.sha256()
    size = 0
    stream.seek(0)
    while chunk := stream.read(HASH_CHUNK_BYTES):
        size += len(chunk)
        digest.update(chunk)
    stream.seek(0)
    return size, digest.hexdigest()


@asynccontextmanager
async def receive_upload(
    request: Request, max_bytes: int, overhead_bytes: int
) -> AsyncIterator[ReceivedFile]:
    check_declared_length(request.headers.get(CONTENT_LENGTH_HEADER), max_bytes, overhead_bytes)
    receive = request._receive
    total = 0
    body_limit = max_bytes + overhead_bytes

    async def bounded_receive() -> Message:
        nonlocal total
        message = await receive()
        total += len(message.get("body", b""))
        if total > body_limit:
            raise AppError(ErrorCode.PAYLOAD_TOO_LARGE)
        return message

    # Starlette's multipart parser consumes Request._receive; bound it while the form is parsed.
    request._receive = bounded_receive
    try:
        form = await request.form(max_files=FORM_MAX_FILES, max_fields=FORM_MAX_FIELDS)
    finally:
        request._receive = receive
    try:
        upload = form.get(FILE_FIELD)
        if not isinstance(upload, UploadFile):
            raise _field_error(FILE_FIELD, "REQUIRED")
        name = normalize_file_name(upload.filename or "")
        content_type = content_type_for(name)
        size, sha256 = await asyncio.to_thread(_measure, upload.file)
        if size > max_bytes:
            raise AppError(ErrorCode.PAYLOAD_TOO_LARGE)
        if size == 0:
            raise _field_error(FILE_FIELD, "EMPTY")
        yield ReceivedFile(name=name, content_type=content_type, size=size, sha256=sha256, stream=upload.file)
    finally:
        await form.close()
