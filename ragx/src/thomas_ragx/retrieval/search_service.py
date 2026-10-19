import asyncio
from collections.abc import Iterator, Sequence
from contextlib import contextmanager
from dataclasses import dataclass
from typing import Any, Final, Protocol
from uuid import UUID

from opentelemetry.trace import Span, StatusCode, Tracer
from opentelemetry.util.types import Attributes
from qdrant_client.models import ScoredPoint

from thomas_ragx.db.pool import Pool
from thomas_ragx.kb.model import KnowledgeBase
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.tracing import (
    ATTR_INPUT_VALUE,
    ATTR_SPAN_KIND,
    SPAN_KIND_RERANKER,
    SPAN_KIND_RETRIEVER,
)
from thomas_ragx.providers.embedding import Embedder
from thomas_ragx.registry.model import Capability, ModelHandle, Role
from thomas_ragx.retrieval.qdrant_store import (
    FIELD_FILE,
    FIELD_VERSION,
    QdrantStore,
    any_condition,
    tenant_filter,
)
from thomas_ragx.retrieval.rerank import Reranker
from thomas_ragx.retrieval.sparse import SparseEncoder

FIELD_DOC_TYPE: Final = "doc_type"
LABEL_FORMAT: Final = "[{source}.{block}]"
SEARCHABLE_STATUSES: Final = ["ready", "degraded"]
ATTR_CANDIDATES: Final = "retrieval.candidates"
ATTR_RESULTS: Final = "retrieval.results"


class ModelSource(Protocol):
    async def model_handle(self, model_id: UUID) -> ModelHandle: ...
    async def role_handle(self, role: Role) -> ModelHandle: ...


@dataclass(frozen=True)
class SearchFilters:
    file_ids: tuple[UUID, ...] = ()
    doc_types: tuple[str, ...] = ()


@dataclass(frozen=True)
class LabelledBlock:
    label: str
    block_id: str
    text: str
    locator: dict[str, Any]


@dataclass(frozen=True)
class SearchResult:
    source: int
    chunk_id: str
    file_id: UUID
    file_name: str
    file_version: int
    node_id: UUID
    section: tuple[str, ...]
    page_start: int | None
    page_end: int | None
    score: float
    blocks: tuple[LabelledBlock, ...]


@dataclass(frozen=True)
class SearchAnswer:
    results: list[SearchResult]
    insufficient_evidence: bool


def _result(source: int, point: ScoredPoint, score: float) -> SearchResult:
    payload = point.payload or {}
    text = str(payload["text"])
    blocks = tuple(
        LabelledBlock(
            LABEL_FORMAT.format(source=source, block=k),
            str(b["block_id"]),
            text[int(b["char_start"]) : int(b["char_end"])],
            dict(b["locator"]),
        )
        for k, b in enumerate(payload["blocks"], start=1)
    )
    return SearchResult(
        source=source,
        chunk_id=str(payload["chunk_id"]),
        file_id=UUID(str(payload["file_id"])),
        file_name=str(payload["file_name"]),
        file_version=int(payload[FIELD_VERSION]),
        node_id=UUID(str(payload["node_id"])),
        section=tuple(payload.get("section") or ()),
        page_start=payload.get("page_start"),
        page_end=payload.get("page_end"),
        score=score,
        blocks=blocks,
    )


@contextmanager
def _safe_span(tracer: Tracer, name: str, attributes: Attributes) -> Iterator[Span]:
    # Dependencies may echo content in exception messages and tracebacks, even with content tracing off.
    with tracer.start_as_current_span(
        name, attributes=attributes, record_exception=False, set_status_on_exception=False
    ) as span:
        try:
            yield span
        except Exception:
            span.set_status(StatusCode.ERROR)
            raise


class SearchService:
    """Hybrid retrieval with required rerank, live-file checks and labelled evidence (C1-C6)."""

    def __init__(
        self,
        pool: Pool,
        models: ModelSource,
        embedder: Embedder,
        sparse: SparseEncoder,
        store: QdrantStore,
        reranker: Reranker,
        tracer: Tracer,
        *,
        dense_limit: int,
        sparse_limit: int,
        fused_limit: int,
        result_limit: int,
        min_score: float,
        trace_content: bool,
    ) -> None:
        self._pool = pool
        self._models = models
        self._embedder = embedder
        self._sparse = sparse
        self._store = store
        self._reranker = reranker
        self._tracer = tracer
        self._dense_limit = dense_limit
        self._sparse_limit = sparse_limit
        self._fused_limit = fused_limit
        self._result_limit = result_limit
        self._min_score = min_score
        self._trace_content = trace_content

    async def _rerank_model(self) -> ModelHandle:
        model = await self._models.role_handle(Role.RERANK)  # LLM_ROLE_NOT_ASSIGNED when unset (ruling C1)
        if Capability.RERANK not in model.record.capabilities:
            raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)  # Jev (decision) reranks from M6
        return model

    async def _live(self, org_id: str, points: list[ScoredPoint]) -> list[ScoredPoint]:
        ids = list({str((p.payload or {})["file_id"]) for p in points})
        if not ids:
            return []
        async with self._pool.connection() as conn:
            rows = await (
                await conn.execute(
                    b"SELECT id FROM kb_files WHERE org_id = %s AND id = ANY(%s) AND deleted_at IS NULL "
                    b"AND status = ANY(%s)",
                    (org_id, ids, SEARCHABLE_STATUSES),
                )
            ).fetchall()
        live = {str(row["id"]) for row in rows}
        return [p for p in points if str((p.payload or {})["file_id"]) in live]  # ruling C3

    async def search(
        self, org_id: str, kb: KnowledgeBase, query: str, filters: SearchFilters
    ) -> SearchAnswer:
        with _safe_span(
            self._tracer, "rag.retrieval.search", {ATTR_SPAN_KIND: SPAN_KIND_RETRIEVER, "kb_id": str(kb.id)}
        ) as span:
            if self._trace_content:
                span.set_attribute(ATTR_INPUT_VALUE, query)
            rerank_model = await self._rerank_model()
            embedding_model = await self._models.model_handle(kb.embedding_model_id)
            dense = (await self._embedder.embed(embedding_model, [query]))[0]
            sparse = await asyncio.to_thread(self._sparse.encode_query, query)
            conditions = []
            if filters.file_ids:
                conditions.append(any_condition(FIELD_FILE, [str(f) for f in filters.file_ids]))
            if filters.doc_types:
                conditions.append(any_condition(FIELD_DOC_TYPE, list(filters.doc_types)))
            points = await self._store.hybrid_query(
                kb.collection_name,
                tenant_filter(org_id, kb.id, *conditions),
                dense,
                sparse,
                dense_limit=self._dense_limit,
                sparse_limit=self._sparse_limit,
                limit=self._fused_limit,
            )
            candidates = await self._live(org_id, points)
            span.set_attribute(ATTR_CANDIDATES, len(candidates))
            if not candidates:
                return SearchAnswer([], insufficient_evidence=True)
            kept = await self._rerank(rerank_model, query, candidates)
            span.set_attribute(ATTR_RESULTS, len(kept))
            results = [_result(n, point, score) for n, (point, score) in enumerate(kept, start=1)]
            return SearchAnswer(results, insufficient_evidence=not results)

    async def _rerank(
        self, model: ModelHandle, query: str, candidates: Sequence[ScoredPoint]
    ) -> list[tuple[ScoredPoint, float]]:
        with _safe_span(self._tracer, "rag.retrieval.rerank", {ATTR_SPAN_KIND: SPAN_KIND_RERANKER}):
            scores = await self._reranker.rerank(
                model, query, [str((c.payload or {})["text"]) for c in candidates], self._result_limit
            )
        ranked = sorted(scores, key=lambda s: s.score, reverse=True)
        return [(candidates[s.index], s.score) for s in ranked if s.score >= self._min_score][
            : self._result_limit
        ]
