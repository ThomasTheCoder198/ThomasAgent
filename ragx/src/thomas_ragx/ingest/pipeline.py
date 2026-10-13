from collections.abc import Awaitable
from pathlib import Path
from time import perf_counter
from typing import Final, Protocol, TypeVar

import structlog
from opentelemetry.trace import Tracer

from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest import repository as ingest
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.outline import build_outline
from thomas_ragx.ingest.repository import IngestTarget
from thomas_ragx.ingest.source import extension_of, fetch_verified
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.parsing.model import ParsedDocument
from thomas_ragx.parsing.service import ParseService
from thomas_ragx.platform.tracing import ATTR_SPAN_KIND, SPAN_KIND_CHAIN

T = TypeVar("T")
STEP_FETCH: Final = "fetch"
STEP_PARSE: Final = "parse"
STEP_PERSIST: Final = "persist"
OUTCOME_FAILED: Final = "failed"
MILLISECONDS_PER_SECOND: Final = 1000
log = structlog.get_logger(__name__)


class IngestRecorder(Protocol):
    def observe_ingest_step(self, step: str, duration_seconds: float) -> None: ...
    def observe_ingest_file(self, outcome: str) -> None: ...


class IngestPipeline:
    """Claim, verify, parse and persist uploads; M1.3b adds indexing."""

    def __init__(
        self,
        pool: Pool,
        storage: ObjectStorage,
        parser: ParseService,
        tracer: Tracer,
        *,
        work_dir: Path,
        lease_seconds: float,
        metrics: IngestRecorder | None = None,
    ) -> None:
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
                await self._run(target)
            except Exception:
                async with self._pool.connection() as conn:
                    await ingest.release_claim(conn, target.file_id, target.version)  # ruling A24
                if self._metrics is not None:
                    self._metrics.observe_ingest_file(OUTCOME_FAILED)
                raise  # the consumer retries or dead-letters; the hook marks the file failed

    async def _run(self, target: IngestTarget) -> None:
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
        await self._step(STEP_PERSIST, self._persist(target, parsed), target)
        async with self._pool.connection() as conn, conn.transaction():
            finished = await ingest.finish(conn, target, parsed, chunk_count=0)
        status = "degraded" if parsed.degraded else "ready"
        if self._metrics is not None and finished:
            self._metrics.observe_ingest_file(status)
        log.info(
            "ingest finished",
            status=status if finished else "superseded",
            provider=parsed.provider.value,
            degraded=parsed.degraded,
            page_count=parsed.page_count,
            block_count=len(parsed.blocks),
        )

    async def _persist(self, target: IngestTarget, parsed: ParsedDocument) -> None:
        outline = build_outline(target.file_id, target.version, target.name, parsed.blocks)
        async with self._pool.connection() as conn, conn.transaction():
            await ingest.replace_outline(conn, target, outline, parsed.blocks)
