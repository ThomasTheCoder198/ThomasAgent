from dataclasses import asdict
from datetime import datetime
from pathlib import Path
from typing import Annotated, Any, Final
from uuid import UUID

from psycopg.rows import DictRow
from pydantic import AfterValidator, Field, StrictInt, StringConstraints

from thomas_ragx.kb.dto import CamelModel
from thomas_ragx.retrieval.search_service import SearchAnswer

# Fixed by the contract (ragx.v1.yaml), not tunables.
QUERY_MAX_LENGTH: Final = 1000
FILE_FILTER_MAX_ITEMS: Final = 20
DOC_TYPE_FILTER_MAX_ITEMS: Final = 12
DOC_TYPE_MAX_LENGTH: Final = 12
NAME_FILTER_MAX_LENGTH: Final = 200
DOC_TYPE_INVALID_MESSAGE: Final = "docTypes entries must be alphanumeric file extensions"

Query = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=QUERY_MAX_LENGTH)]
DocType = Annotated[
    str, StringConstraints(strip_whitespace=True, min_length=1, max_length=DOC_TYPE_MAX_LENGTH)
]


def _stored_doc_type(value: str) -> str:
    # Ingest stores doc_type as the lowercase extension without its dot, so only that form matches in Qdrant.
    normalized = value.strip().lstrip(".").lower()
    if not normalized.isalnum():
        raise ValueError(DOC_TYPE_INVALID_MESSAGE)
    return normalized


StoredDocType = Annotated[DocType, AfterValidator(_stored_doc_type)]


class KbSearchFilters(CamelModel):
    file_ids: list[UUID] | None = Field(default=None, max_length=FILE_FILTER_MAX_ITEMS)
    doc_types: list[StoredDocType] | None = Field(default=None, max_length=DOC_TYPE_FILTER_MAX_ITEMS)


class KbSearchInput(CamelModel):
    """Closed (extra="forbid"): a kbId/orgId/tenantId in the body is VALIDATION_FAILED (spec §3.3 item 5)."""

    query: Query
    filters: KbSearchFilters | None = None


class KbOutlineInput(CamelModel):
    file_id: UUID


class KbReadSectionInput(CamelModel):
    node_id: UUID


class KbListDocumentsInput(CamelModel):
    name_contains: Annotated[str, StringConstraints(max_length=NAME_FILTER_MAX_LENGTH)] | None = None
    doc_type: DocType | None = None
    cursor: str | None = None
    limit: StrictInt | None = None


class LabelledBlockResponse(CamelModel):
    label: str
    block_id: str
    text: str
    locator: dict[str, Any]


class SearchResultResponse(CamelModel):
    source: int
    chunk_id: str
    file_id: UUID
    file_name: str
    node_id: UUID
    section: list[str]
    page_start: int | None
    page_end: int | None
    score: float
    blocks: list[LabelledBlockResponse]


class KbSearchOutput(CamelModel):
    results: list[SearchResultResponse]
    insufficient_evidence: bool


class OutlineNodeResponse(CamelModel):
    node_id: UUID
    parent_id: UUID | None
    depth: int
    title: str
    summary: str


class KbOutlineOutput(CamelModel):
    file_id: UUID
    file_name: str
    nodes: list[OutlineNodeResponse]
    truncated: bool


class SectionBlockResponse(CamelModel):
    block_id: str
    type: str
    text: str
    locator: dict[str, Any]


class KbReadSectionOutput(CamelModel):
    node_id: UUID
    file_id: UUID
    file_name: str
    title: str
    path: list[str]
    blocks: list[SectionBlockResponse]
    truncated: bool


class DocumentResponse(CamelModel):
    file_id: UUID
    name: str
    doc_type: str
    version: int
    status: str
    page_count: int | None
    created_at: datetime


def search_output(answer: SearchAnswer) -> KbSearchOutput:
    return KbSearchOutput.model_validate(asdict(answer))


def outline_output(file_id: UUID, name: str, nodes: list[DictRow], truncated: bool) -> KbOutlineOutput:
    return KbOutlineOutput(
        file_id=file_id,
        file_name=name,
        truncated=truncated,
        nodes=[
            OutlineNodeResponse(
                node_id=row["id"],
                parent_id=row["parent_id"],
                depth=row["depth"],
                title=row["title"],
                summary=row["summary"] or "",
            )
            for row in nodes
        ],
    )


def section_output(section: dict[str, Any]) -> KbReadSectionOutput:
    node = section["node"]
    return KbReadSectionOutput(
        node_id=node["id"],
        file_id=node["file_id"],
        file_name=node["file_name"],
        title=node["title"],
        path=section["path"],
        truncated=section["truncated"],
        blocks=[
            SectionBlockResponse(
                block_id=row["block_id"], type=row["block_type"], text=row["text"], locator=row["locator"]
            )
            for row in section["blocks"]
        ],
    )


def document_response(row: DictRow) -> DocumentResponse:
    return DocumentResponse(
        file_id=row["id"],
        name=row["name"],
        doc_type=Path(row["name"]).suffix.lstrip(".").lower(),
        version=row["version"],
        status=row["status"],
        page_count=row["page_count"],
        created_at=row["created_at"],
    )
