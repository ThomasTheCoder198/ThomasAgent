from collections.abc import Sequence
from dataclasses import dataclass
from datetime import datetime
from typing import Final
from uuid import UUID

import structlog
from qdrant_client import AsyncQdrantClient
from qdrant_client.http.exceptions import UnexpectedResponse
from qdrant_client.models import (
    Distance,
    FieldCondition,
    Filter,
    FilterSelector,
    Fusion,
    FusionQuery,
    KeywordIndexParams,
    KeywordIndexType,
    MatchAny,
    MatchValue,
    Modifier,
    PointStruct,
    Prefetch,
    ScoredPoint,
    SparseVector,
    SparseVectorParams,
    VectorParams,
)

from thomas_ragx.ingest.chunker import Chunk, point_id_for

DENSE_VECTOR: Final = "dense"
SPARSE_VECTOR: Final = "bm25"
FIELD_ORG: Final = "org_id"
FIELD_KB: Final = "kb_id"
FIELD_FILE: Final = "file_id"
TENANT_FIELDS: Final = (FIELD_ORG, FIELD_KB)
HTTP_CONFLICT: Final = 409
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class PointOwner:
    org_id: str
    kb_id: UUID
    file_id: UUID
    version: int


def _match(field: str, value: str) -> FieldCondition:
    return FieldCondition(key=field, match=MatchValue(value=value))


def tenant_filter(org_id: str, kb_id: UUID | str, *conditions: FieldCondition) -> Filter:
    """The only way to build a Qdrant filter in ragx (ruling B2): org and KB are never optional."""
    if not org_id or not str(kb_id):
        raise ValueError("a Qdrant filter needs both org_id and kb_id")
    return Filter(must=[_match(FIELD_ORG, org_id), _match(FIELD_KB, str(kb_id)), *conditions])


def any_condition(field: str, values: Sequence[str]) -> FieldCondition:
    return FieldCondition(key=field, match=MatchAny(any=list(values)))


def file_condition(file_id: UUID | str) -> FieldCondition:
    return _match(FIELD_FILE, str(file_id))


def build_point(
    owner: PointOwner,
    chunk: Chunk,
    dense: Sequence[float],
    sparse: SparseVector,
    file_name: str,
    doc_type: str,
    created_at: datetime,
) -> PointStruct:
    return PointStruct(
        id=str(point_id_for(chunk.chunk_id)),
        vector={DENSE_VECTOR: list(dense), SPARSE_VECTOR: sparse},
        payload={
            FIELD_ORG: owner.org_id,
            FIELD_KB: str(owner.kb_id),
            FIELD_FILE: str(owner.file_id),
            "version": owner.version,
            "chunk_id": chunk.chunk_id,
            "chunk_index": chunk.index,
            "node_id": str(chunk.node_id),
            "section": list(chunk.section),
            "page_start": chunk.page_start,
            "page_end": chunk.page_end,
            "blocks": [
                {
                    "block_id": b.block_id,
                    "locator": b.locator.to_json(),
                    "char_start": b.char_start,
                    "char_end": b.char_end,
                }
                for b in chunk.blocks
            ],
            "doc_type": doc_type,
            "file_name": file_name,
            "lang": chunk.lang,
            "text": chunk.text,
            "created_at": created_at.isoformat(),
        },
    )


class QdrantStore:
    """One collection per embedding model and dims (main spec §4.2 step 7); tenants by payload (ruling B1)."""

    def __init__(self, client: AsyncQdrantClient, *, upsert_batch_size: int) -> None:
        self._client = client
        self._batch = upsert_batch_size

    async def ensure_collection(self, name: str, dims: int) -> None:
        if not await self._client.collection_exists(name):
            try:
                await self._client.create_collection(
                    name,
                    vectors_config={DENSE_VECTOR: VectorParams(size=dims, distance=Distance.COSINE)},
                    sparse_vectors_config={SPARSE_VECTOR: SparseVectorParams(modifier=Modifier.IDF)},
                )
            except UnexpectedResponse as exc:
                if exc.status_code != HTTP_CONFLICT:  # another worker created it first (ruling B4)
                    raise
        for field in TENANT_FIELDS:
            await self._client.create_payload_index(
                name, field, field_schema=KeywordIndexParams(type=KeywordIndexType.KEYWORD, is_tenant=True)
            )
        await self._client.create_payload_index(
            name, FIELD_FILE, field_schema=KeywordIndexParams(type=KeywordIndexType.KEYWORD)
        )

    async def _delete(self, name: str, flt: Filter) -> None:
        if await self._client.collection_exists(name):
            await self._client.delete(name, points_selector=FilterSelector(filter=flt), wait=True)

    async def delete_file_points(self, name: str, org_id: str, kb_id: UUID, file_id: UUID) -> None:
        await self._delete(name, tenant_filter(org_id, kb_id, file_condition(file_id)))

    async def delete_kb_points(self, name: str, org_id: str, kb_id: UUID) -> None:
        await self._delete(name, tenant_filter(org_id, kb_id))

    async def replace_file_points(self, name: str, owner: PointOwner, points: Sequence[PointStruct]) -> None:
        """Delete then upsert (ruling B3): a re-ingest with fewer chunks leaves no orphans."""
        await self.delete_file_points(name, owner.org_id, owner.kb_id, owner.file_id)
        for start in range(0, len(points), self._batch):
            await self._client.upsert(name, points=list(points[start : start + self._batch]), wait=True)

    async def count(self, name: str, flt: Filter) -> int:
        return (await self._client.count(name, count_filter=flt, exact=True)).count

    async def hybrid_query(
        self,
        name: str,
        flt: Filter,
        dense: Sequence[float],
        sparse: SparseVector,
        *,
        dense_limit: int,
        sparse_limit: int,
        limit: int,
    ) -> list[ScoredPoint]:
        """Dense + BM25 prefetch fused by RRF (main spec §4.3); both legs carry the tenant filter."""
        if not await self._client.collection_exists(name):
            return []  # a KB with nothing indexed yet
        response = await self._client.query_points(
            name,
            prefetch=[
                Prefetch(query=list(dense), using=DENSE_VECTOR, limit=dense_limit, filter=flt),
                Prefetch(query=sparse, using=SPARSE_VECTOR, limit=sparse_limit, filter=flt),
            ],
            query=FusionQuery(fusion=Fusion.RRF),
            query_filter=flt,
            limit=limit,
            with_payload=True,
        )
        return list(response.points)
