import asyncio
import json
from collections.abc import Awaitable
from datetime import UTC, datetime
from pathlib import Path
from time import perf_counter
from typing import Final, Protocol, TypeVar
from uuid import UUID

import structlog
from opentelemetry import trace
from opentelemetry.trace import Tracer
from qdrant_client.models import SparseVector

from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest import repository as ingest
from thomas_ragx.ingest.chunker import Chunk
from thomas_ragx.ingest.indexing import Indexing
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.outline import Outline, build_outline
from thomas_ragx.ingest.repository import IngestTarget
from thomas_ragx.ingest.source import extension_of, fetch_verified
from thomas_ragx.ingest.summaries import SectionText
from thomas_ragx.kb import repository as kbs
from thomas_ragx.kb.model import FileStatus
from thomas_ragx.kb.storage import PREVIEW_BUCKET, ObjectStorage, preview_key
from thomas_ragx.parsing.model import ParsedDocument
from thomas_ragx.parsing.service import ParseService
from thomas_ragx.platform.timing import MILLISECONDS_PER_SECOND
from thomas_ragx.platform.tracing import ATTR_SPAN_KIND, SPAN_KIND_CHAIN
from thomas_ragx.retrieval.qdrant_store import PointOwner, build_point

T = TypeVar("T")
STEP_FETCH: Final = "fetch"
STEP_PARSE: Final = "parse"
STEP_PERSIST: Final = "persist"
STEP_ENRICH: Final = "enrich"
STEP_CHUNK: Final = "chunk"
STEP_EMBED: Final = "embed"
STEP_INDEX: Final = "index"
STEP_PREVIEW: Final = "preview"
PREVIEW_CONTENT_TYPE: Final = "application/json"
OUTCOME_FAILED: Final = "failed"
log = structlog.get_logger(__name__)


class IngestRecorder(Protocol):
    def observe_ingest_step(self, step: str, duration_seconds: float) -> None: ...
    def observe_ingest_file(self, outcome: str) -> None: ...


class IngestPipeline:
    """Claim uploads and build searchable chunks before publishing readiness."""

    def __init__(
        self,
        pool: Pool,
        storage: ObjectStorage,
        parser: ParseService,
        tracer: Tracer,
        *,
        indexing: Indexing,
        work_dir: Path,
        lease_seconds: float,
        metrics: IngestRecorder | None = None,
    ) -> None:
        self._indexing = indexing
        self._pool = pool
        self._storage = storage
        self._parser = parser
        self._tracer = tracer
        self._work_dir = work_dir
        self._lease_seconds = lease_seconds
        self._metrics = metrics

    @staticmethod
    def _span_attributes() -> dict[str, str]:
        return {
            key: str(value)
            for key, value in structlog.contextvars.get_contextvars().items()
            if key in {"request_id", "org_id", "kb_id", "file_id"}
        }

    async def _step(self, name: str, work: Awaitable[T], target: IngestTarget) -> T:
        started = perf_counter()
        try:
            with self._tracer.start_as_current_span(
                f"rag.ingest.{name}",
                record_exception=False,
                set_status_on_exception=False,
                attributes={
                    ATTR_SPAN_KIND: SPAN_KIND_CHAIN,
                    "kb_id": str(target.kb_id),
                    "file_id": str(target.file_id),
                    **self._span_attributes(),
                },
            ):
                result = await work
                if name == STEP_CHUNK and isinstance(result, list):
                    trace.get_current_span().set_attribute("chunk.count", len(result))
        finally:
            elapsed = perf_counter() - started
            if self._metrics is not None:
                self._metrics.observe_ingest_step(name, elapsed)
        log.info(
            "ingest step completed",
            step=name,
            version=target.version,
            duration_ms=round(elapsed * MILLISECONDS_PER_SECOND),
        )
        return result

    async def run(self, job: IngestJob) -> None:
        with structlog.contextvars.bound_contextvars(request_id=job.request_id, org_id=job.org_id):
            await self._run_job(job)

    async def _run_job(self, job: IngestJob) -> None:
        async with self._pool.connection() as conn:
            target = await ingest.claim_file(conn, job, self._lease_seconds)
        if target is None:
            log.info("ingest skipped", file_id=str(job.file_id), version=job.version)
            return
        with structlog.contextvars.bound_contextvars(kb_id=str(target.kb_id), file_id=str(target.file_id)):
            try:
                await self._run(target, job.request_id)
            except Exception:
                async with self._pool.connection() as conn:
                    await ingest.release_claim(conn, target.file_id, target.version)  # ruling A24
                if self._metrics is not None:
                    self._metrics.observe_ingest_file(OUTCOME_FAILED)
                raise  # the consumer retries or dead-letters; the hook marks the file failed

    async def _run(self, target: IngestTarget, request_id: str) -> None:
        path = await self._step(STEP_FETCH, fetch_verified(self._storage, target, self._work_dir), target)
        try:
            parsed = await self._step(
                STEP_PARSE,
                self._parser.parse(
                    path,
                    extension_of(target),
                    external_parsing=target.external_parsing,
                    span_attributes=self._span_attributes(),
                ),
                target,
            )
        finally:
            path.unlink(missing_ok=True)
        outline = build_outline(target.file_id, target.version, target.name, parsed.blocks)
        persisted = await self._step(STEP_PERSIST, self._persist(target, outline, parsed), target)
        if not persisted:
            log.info("ingest abandoned, target deleted", version=target.version)
            return
        await self._advance(target, FileStatus.ENRICHING)
        await self._step(STEP_ENRICH, self._enrich(outline, parsed), target)
        await self._advance(target, FileStatus.CHUNKING)
        chunks = await self._step(
            STEP_CHUNK,
            asyncio.to_thread(
                self._indexing.chunker.chunk,
                target.file_id,
                target.version,
                target.name,
                parsed.blocks,
                outline,
            ),
            target,
        )
        await self._advance(target, FileStatus.EMBEDDING)
        dense, sparse = await self._step(STEP_EMBED, self._embed(target, chunks), target)
        await self._advance(target, FileStatus.INDEXING)
        await self._publish(target, outline, parsed, chunks, dense, sparse, request_id)

    async def _publish(
        self,
        target: IngestTarget,
        outline: Outline,
        parsed: ParsedDocument,
        chunks: list[Chunk],
        dense: list[list[float]],
        sparse: list[SparseVector],
        request_id: str,
    ) -> None:
        # KB before file rows, matching purge/confirm: no writer can outlive a completed purge.
        async with self._pool.connection() as conn, conn.transaction():
            await kbs.lock_collection(conn, target.org_id, target.kb_id)
            if not await ingest.is_live(conn, target.file_id, target.version):
                await self._discard(target)
                log.info("ingest abandoned, target deleted", version=target.version)
                return
            try:
                await self._step(STEP_INDEX, self._index(target, chunks, dense, sparse), target)
                await self._step(STEP_PREVIEW, self._preview(target, outline, parsed), target)
                finished = await ingest.finish(conn, target, parsed, chunk_count=len(chunks))
                retired = await ingest.supersede_older_versions(conn, target, request_id) if finished else []
                if not finished:
                    await self._discard(target)
            except Exception:
                # A failed later batch or preview must not strand earlier external writes.
                await self._discard(target)
                raise
        self._finished(parsed, finished, len(parsed.blocks), len(chunks), len(retired))

    async def _discard(self, target: IngestTarget) -> None:
        await self._indexing.store.delete_file_points(
            target.collection_name, target.org_id, target.kb_id, target.file_id
        )
        await self._storage.remove(PREVIEW_BUCKET, preview_key(target.org_id, target.kb_id, target.file_id))

    async def _advance(self, target: IngestTarget, status: FileStatus) -> None:
        async with self._pool.connection() as conn:
            await ingest.set_step(conn, target.file_id, status)

    async def _persist(self, target: IngestTarget, outline: Outline, parsed: ParsedDocument) -> bool:
        async with self._pool.connection() as conn, conn.transaction():
            await kbs.lock_collection(conn, target.org_id, target.kb_id)
            if not await ingest.is_live(conn, target.file_id, target.version):
                return False
            await ingest.replace_outline(conn, target, outline, parsed.blocks)
            return True

    async def _enrich(self, outline: Outline, parsed: ParsedDocument) -> None:
        texts: dict[UUID, list[str]] = {}
        for block in parsed.blocks:
            texts.setdefault(outline.block_nodes[block.block_id], []).append(block.text)
        sections = [
            SectionText(node.node_id, "\n".join(texts.get(node.node_id, [])))
            for node in outline.nodes
            if node.depth > 0 and texts.get(node.node_id)
        ]
        summaries = await self._indexing.summarizer.summarize(sections)
        async with self._pool.connection() as conn:
            await ingest.set_summaries(conn, summaries)

    async def _embed(
        self, target: IngestTarget, chunks: list[Chunk]
    ) -> tuple[list[list[float]], list[SparseVector]]:
        model = await self._indexing.models(target.embedding_model_id)  # the KB's model, never the role (B5)
        result = await self._indexing.embedder.embed(
            model, target.embedding_dims, [c.embed_text for c in chunks]
        )
        sparse = await asyncio.to_thread(self._indexing.sparse.encode_passages, [c.text for c in chunks])
        trace.get_current_span().set_attributes(
            {"embedding.cache_hits": result.hits, "embedding.cache_misses": result.misses}
        )
        return result.vectors, sparse

    async def _index(
        self, target: IngestTarget, chunks: list[Chunk], dense: list[list[float]], sparse: list[SparseVector]
    ) -> None:
        owner = PointOwner(target.org_id, target.kb_id, target.file_id, target.version)
        doc_type = extension_of(target).lstrip(".")
        now = datetime.now(UTC)
        points = [
            build_point(owner, c, d, s, target.name, doc_type, now)
            for c, d, s in zip(chunks, dense, sparse, strict=True)
        ]
        await self._indexing.store.ensure_collection(target.collection_name, target.embedding_dims)
        await self._indexing.store.replace_file_points(target.collection_name, owner, points)
        trace.get_current_span().set_attribute("qdrant.points", len(points))

    async def _preview(self, target: IngestTarget, outline: Outline, parsed: ParsedDocument) -> None:
        document = {
            "fileId": str(target.file_id),
            "version": target.version,
            "provider": parsed.provider.value,
            "pageCount": parsed.page_count,
            "blocks": [
                {
                    "blockId": b.block_id,
                    "nodeId": str(outline.block_nodes[b.block_id]),
                    "type": b.block_type.value,
                    "text": b.text,
                    "locator": b.locator.to_json(),
                }
                for b in parsed.blocks
            ],
        }
        data = json.dumps(document, ensure_ascii=False).encode()
        await self._storage.put_bytes(
            PREVIEW_BUCKET,
            preview_key(target.org_id, target.kb_id, target.file_id),
            data,
            PREVIEW_CONTENT_TYPE,
        )

    def _finished(
        self,
        parsed: ParsedDocument,
        finished: bool,
        block_count: int,
        chunk_count: int,
        retired_versions: int,
    ) -> None:
        status = "degraded" if parsed.degraded else "ready"
        if self._metrics is not None and finished:
            self._metrics.observe_ingest_file(status)
        log.info(
            "ingest finished",
            status=status if finished else "superseded",
            provider=parsed.provider.value,
            degraded=parsed.degraded,
            page_count=parsed.page_count,
            block_count=block_count,
            chunk_count=chunk_count,
            retired_versions=retired_versions,
        )
