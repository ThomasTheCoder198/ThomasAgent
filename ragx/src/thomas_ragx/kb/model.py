from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import Final
from uuid import UUID

from thomas_ragx.kb.presign import PresignedPost

# Operators rename it freely; Vietnamese is the product's default language.
DEFAULT_KB_NAME: Final = "Mặc định"


@dataclass(frozen=True)
class Org:
    id: str
    name: str
    external_parsing: bool
    default_kb_id: UUID
    created_at: datetime


@dataclass(frozen=True)
class KnowledgeBase:
    id: UUID
    org_id: str
    name: str
    is_default: bool
    embedding_model: str
    embedding_dims: int
    file_count: int
    created_at: datetime
    updated_at: datetime


class FileStatus(StrEnum):
    UPLOADING = "uploading"
    PENDING = "pending"
    PARSING = "parsing"
    ENRICHING = "enriching"
    CHUNKING = "chunking"
    EMBEDDING = "embedding"
    INDEXING = "indexing"
    READY = "ready"
    FAILED = "failed"
    DEGRADED = "degraded"


@dataclass(frozen=True)
class KbFile:
    id: UUID
    org_id: str
    kb_id: UUID
    name: str
    version: int
    content_type: str
    size_bytes: int
    sha256: str
    status: FileStatus
    error_code: str
    degraded: bool
    created_at: datetime
    updated_at: datetime


@dataclass(frozen=True)
class UploadSlot:
    """A presigned upload that has not been confirmed: its bytes, hash and version are unknown."""

    id: UUID
    org_id: str
    kb_id: UUID
    name: str
    content_type: str
    size_bytes: int
    created_at: datetime


@dataclass(frozen=True)
class UploadTicket:
    file_id: UUID
    post: PresignedPost


@dataclass(frozen=True)
class DownloadLink:
    url: str
    expires_at: datetime
