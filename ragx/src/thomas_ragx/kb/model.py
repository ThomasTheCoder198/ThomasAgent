from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import Final
from uuid import UUID

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
    created_at: datetime
    updated_at: datetime


class FileStatus(StrEnum):
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
