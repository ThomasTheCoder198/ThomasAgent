from datetime import datetime
from typing import Annotated, Any, Final
from uuid import UUID

from pydantic import BaseModel, ConfigDict, StringConstraints
from pydantic.alias_generators import to_camel

from thomas_ragx.kb.model import KbFile, KnowledgeBase, Org

NAME_MAX_LENGTH: Final = 200
DisplayName = Annotated[
    str, StringConstraints(strip_whitespace=True, min_length=1, max_length=NAME_MAX_LENGTH)
]


class CamelModel(BaseModel):
    # Unknown fields are rejected so a body cannot smuggle another org or KB.
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True, extra="forbid")


class OrgProvisionRequest(CamelModel):
    name: DisplayName
    external_parsing: bool


class KnowledgeBaseWrite(CamelModel):
    name: DisplayName


class OrgResponse(CamelModel):
    org_id: str
    name: str
    external_parsing: bool
    default_kb_id: UUID
    created_at: datetime

    @classmethod
    def from_model(cls, org: Org) -> "OrgResponse":
        return cls(
            org_id=org.id,
            name=org.name,
            external_parsing=org.external_parsing,
            default_kb_id=org.default_kb_id,
            created_at=org.created_at,
        )


class KnowledgeBaseResponse(CamelModel):
    id: UUID
    name: str
    is_default: bool
    embedding_model: str
    embedding_dims: int
    file_count: int
    created_at: datetime
    updated_at: datetime

    @classmethod
    def from_model(cls, kb: KnowledgeBase) -> "KnowledgeBaseResponse":
        return cls(
            id=kb.id,
            name=kb.name,
            is_default=kb.is_default,
            embedding_model=kb.embedding_model,
            embedding_dims=kb.embedding_dims,
            file_count=kb.file_count,
            created_at=kb.created_at,
            updated_at=kb.updated_at,
        )


def to_json(model: BaseModel) -> dict[str, Any]:
    return model.model_dump(by_alias=True, mode="json")


class FileResponse(CamelModel):
    id: UUID
    kb_id: UUID
    name: str
    version: int
    content_type: str
    size_bytes: int
    sha256: str
    status: str
    error_code: str
    degraded: bool
    created_at: datetime
    updated_at: datetime

    @classmethod
    def from_model(cls, file: KbFile) -> "FileResponse":
        return cls(
            id=file.id,
            kb_id=file.kb_id,
            name=file.name,
            version=file.version,
            content_type=file.content_type,
            size_bytes=file.size_bytes,
            sha256=file.sha256,
            status=file.status.value,
            error_code=file.error_code,
            degraded=file.degraded,
            created_at=file.created_at,
            updated_at=file.updated_at,
        )
