import unicodedata
from collections.abc import Mapping
from pathlib import PurePosixPath
from typing import Final

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

MAX_NAME_LENGTH: Final = 255
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


def field_error(field: str, code: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": code}]})


def normalize_file_name(raw: str, field: str) -> str:
    # NFC so a macOS (NFD) upload and a Windows (NFC) upload of the same name are one file with two versions.
    normalized = unicodedata.normalize("NFC", raw)
    if any(unicodedata.category(char) == "Cc" for char in normalized):
        raise field_error(field, "INVALID_NAME")
    name = normalized.replace("\\", "/").rsplit("/", 1)[-1].strip()
    if not name or name in RESERVED_NAMES or len(name) > MAX_NAME_LENGTH:
        raise field_error(field, "INVALID_NAME")
    return name


def content_type_for(name: str) -> str:
    content_type = SUPPORTED_EXTENSIONS.get(PurePosixPath(name).suffix.lower())
    if content_type is None:
        raise AppError(ErrorCode.UNSUPPORTED_MEDIA_TYPE)
    return content_type
