"""Two attempts of one file: the lease of the first expires and a second attempt takes over (Codex I1)."""

import asyncio
import dataclasses
import uuid
from collections.abc import Sequence
from typing import Any
from unittest.mock import Mock

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient

from tests.integration.ingest_support import MARKDOWN, ORG, REQUEST_ID, file_row, pipeline_for, uploaded_file
from tests.rag_fakes import HashEmbedder
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest import pipeline as pipeline_module
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.ingest.repository import claim_file
from thomas_ragx.kb.storage import PREVIEW_BUCKET, ObjectStorage
from thomas_ragx.parsing.model import ParsedDocument
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import ModelHandle
from thomas_ragx.retrieval.qdrant_store import file_condition, tenant_filter
from thomas_ragx.worker.consumer import StreamMessage
from thomas_ragx.worker.handlers import IngestDeadLetter

pytestmark = pytest.mark.integration
STALE_MARKER = "STALE-ATTEMPT"
WAIT_SECONDS = 30
DEADLINE_SECONDS = 3.0


def _job(kb_id: str, file_id: str) -> IngestJob:
    return IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 1, REQUEST_ID)


class Gate:
    def __init__(self) -> None:
        self.reached = asyncio.Event()
        self.release = asyncio.Event()

    async def hold(self) -> None:
        self.reached.set()
        await self.release.wait()


class PausedEmbedder(HashEmbedder):
    def __init__(self, gate: Gate, fail_after: bool = False) -> None:
        super().__init__(1536)
        self._gate = gate
        self._fail_after = fail_after

    async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
        await self._gate.hold()
        if self._fail_after:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
        return await super().embed(model, texts)


def _pause_after_parse(pipeline: IngestPipeline, gate: Gate) -> None:
    parse = pipeline._parser.parse

    async def paused(*args: Any, **kwargs: Any) -> ParsedDocument:
        parsed = await parse(*args, **kwargs)
        await gate.hold()
        # A re-parse may differ (Datalab drifts between runs); the stale text must never replace the owner's.
        blocks = tuple(dataclasses.replace(b, text=f"{b.text} {STALE_MARKER}") for b in parsed.blocks)
        return dataclasses.replace(parsed, blocks=blocks)

    pipeline._parser.parse = paused  # type: ignore[method-assign]


async def _expire_lease(pool: Pool, file_id: str) -> None:
    async with pool.connection() as conn:
        await conn.execute(
            b"UPDATE kb_files SET ingest_lease_until = now() - interval '1 second' WHERE id = %s", (file_id,)
        )


async def _points(qdrant: AsyncQdrantClient, pool: Pool, kb_id: str, file_id: str) -> list[Any]:
    async with pool.connection() as conn:
        row = await (
            await conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,))
        ).fetchone()
    assert row is not None
    if not await qdrant.collection_exists(row["collection_name"]):
        return []
    found, _ = await qdrant.scroll(
        row["collection_name"], scroll_filter=tenant_filter(ORG, kb_id, file_condition(file_id)), limit=1000
    )
    return found


async def _block_texts(pool: Pool, file_id: str) -> list[str]:
    async with pool.connection() as conn:
        rows = await (
            await conn.execute(
                b"SELECT text FROM doc_blocks WHERE file_id = %s ORDER BY position", (file_id,)
            )
        ).fetchall()
    return [row["text"] for row in rows]


async def _preview_size(minio: Minio, kb_id: str, file_id: str) -> int | None:
    return await ObjectStorage(minio).size_of(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json")


async def _assert_ready_and_searchable(
    pool: Pool, minio: Minio, qdrant: AsyncQdrantClient, kb_id: str, file_id: str
) -> None:
    row = await file_row(pool, file_id)
    points = await _points(qdrant, pool, kb_id, file_id)
    assert row["status"] == "ready"
    assert row["chunk_count"] == len(points) > 0
    assert await _preview_size(minio, kb_id, file_id) is not None
    assert not any(STALE_MARKER in text for text in await _block_texts(pool, file_id))


@pytest.mark.parametrize("pause_in", ["parse", "embed"])
async def test_a_stale_attempt_cannot_undo_the_file_its_successor_finished(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
    pause_in: str,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "fenced.md", MARKDOWN)
    gate = Gate()
    stale = pipeline_for(
        pool, minio, qdrant, tmp_path / "a", embedder=PausedEmbedder(gate) if pause_in == "embed" else None
    )
    if pause_in == "parse":
        _pause_after_parse(stale, gate)
    attempt_a = asyncio.create_task(stale.run(_job(kb_id, file_id)))
    try:
        await asyncio.wait_for(gate.reached.wait(), WAIT_SECONDS)
        await _expire_lease(pool, file_id)
        await pipeline_for(pool, minio, qdrant, tmp_path / "b").run(_job(kb_id, file_id))
        await _assert_ready_and_searchable(pool, minio, qdrant, kb_id, file_id)
    finally:
        gate.release.set()
    await asyncio.wait_for(attempt_a, WAIT_SECONDS)  # a lost attempt ends quietly; nothing to retry
    await _assert_ready_and_searchable(pool, minio, qdrant, kb_id, file_id)


async def test_a_stale_attempt_stops_before_its_index_once_its_successor_finished(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    # Before I1 the stale attempt reached its index, and that index's failure path erased B's points.
    kb_id, file_id = await uploaded_file(pool, minio_config, "fenced-fail.md", MARKDOWN)
    gate = Gate()
    stale = pipeline_for(pool, minio, qdrant, tmp_path / "a", embedder=PausedEmbedder(gate))
    index_calls: list[str] = []

    async def failing_index(collection: str, *_: Any) -> None:
        index_calls.append(collection)
        raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)

    stale._indexing.store.replace_file_points = failing_index  # type: ignore[method-assign]
    attempt_a = asyncio.create_task(stale.run(_job(kb_id, file_id)))
    try:
        await asyncio.wait_for(gate.reached.wait(), WAIT_SECONDS)
        await _expire_lease(pool, file_id)
        await pipeline_for(pool, minio, qdrant, tmp_path / "b").run(_job(kb_id, file_id))
    finally:
        gate.release.set()
    await asyncio.wait_for(asyncio.gather(attempt_a, return_exceptions=True), WAIT_SECONDS)
    assert index_calls == []  # it read claim=lost under the KB lock and wrote nothing
    await _assert_ready_and_searchable(pool, minio, qdrant, kb_id, file_id)


async def test_an_owner_whose_finish_loses_to_a_takeover_removes_only_its_own_writes(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "owned-then-lost.md", MARKDOWN)
    indexed, resume = asyncio.Event(), asyncio.Event()
    owner = pipeline_for(pool, minio, qdrant, tmp_path / "a")
    replace = owner._indexing.store.replace_file_points

    async def paused_index(collection: str, point_owner: Any, points: list[Any]) -> None:
        await replace(collection, point_owner, points)
        indexed.set()
        await resume.wait()

    owner._indexing.store.replace_file_points = paused_index  # type: ignore[method-assign]
    attempt_a = asyncio.create_task(owner.run(_job(kb_id, file_id)))
    try:
        await asyncio.wait_for(indexed.wait(), WAIT_SECONDS)
        assert await _points(qdrant, pool, kb_id, file_id)  # A owned the file at its check and indexed
        await _expire_lease(pool, file_id)
        async with pool.connection() as conn:  # B takes over; claims do not wait for the KB lock A holds
            successor = await claim_file(conn, _job(kb_id, file_id), 600.0)
        assert successor is not None
    finally:
        resume.set()
    await asyncio.wait_for(attempt_a, WAIT_SECONDS)  # A's finish loses to B's token; A ends quietly
    row = await file_row(pool, file_id)
    assert (row["status"], row["ingest_attempt_id"]) == ("parsing", successor.attempt_id)
    # Nobody else could publish while A held the KB lock, so A's cleanup removed only A's own writes.
    assert await _points(qdrant, pool, kb_id, file_id) == []
    assert await _preview_size(minio, kb_id, file_id) is None
    await _expire_lease(pool, file_id)  # B's work then completes as a normal attempt
    await pipeline_for(pool, minio, qdrant, tmp_path / "b").run(_job(kb_id, file_id))
    await _assert_ready_and_searchable(pool, minio, qdrant, kb_id, file_id)


async def test_a_failing_stale_attempt_neither_releases_nor_fails_its_successors_claim(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "fenced-claim.md", MARKDOWN)
    gate = Gate()
    stale = pipeline_for(pool, minio, qdrant, tmp_path, embedder=PausedEmbedder(gate, fail_after=True))
    attempt_a = asyncio.create_task(stale.run(_job(kb_id, file_id)))
    try:
        await asyncio.wait_for(gate.reached.wait(), WAIT_SECONDS)
        await _expire_lease(pool, file_id)
        async with pool.connection() as conn:
            successor = await claim_file(conn, _job(kb_id, file_id), 600.0)  # B is mid-ingest
        assert successor is not None
    finally:
        gate.release.set()
    with pytest.raises(AppError):
        await asyncio.wait_for(attempt_a, WAIT_SECONDS)
    row = await file_row(pool, file_id)
    assert row["status"] == "parsing" and row["ingest_lease_until"] is not None
    assert row["ingest_attempt_id"] == successor.attempt_id
    # A's last delivery dead-letters; the hook must not fail the row B still works on.
    message = StreamMessage(
        stream=STREAM_INGEST_REQUESTED,
        entry_id="1-0",
        outbox_id="",
        org_id=ORG,
        request_id=REQUEST_ID,
        payload={"kb_id": kb_id, "file_id": file_id},
        deliveries=3,
    )
    await IngestDeadLetter(pool)(message, ErrorCode.PROVIDER_UNAVAILABLE.value)
    assert (await file_row(pool, file_id))["status"] == "parsing"


async def test_the_job_deadline_discards_a_partial_index_and_releases_the_claim(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "deadline.md", MARKDOWN)
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path, job_timeout_seconds=DEADLINE_SECONDS)
    store = pipeline._indexing.store
    first_batch = asyncio.Event()

    async def hanging(collection: str, owner: Any, points: list[Any]) -> None:
        await qdrant.upsert(collection, points[:1], wait=True)
        first_batch.set()
        await asyncio.Event().wait()  # a stuck later batch: only the deadline ends it

    store.replace_file_points = hanging  # type: ignore[method-assign]
    log = Mock()  # the module logger may already be cached by earlier tests, so capture_logs cannot see it
    monkeypatch.setattr(pipeline_module, "log", log)
    with pytest.raises(AppError) as raised:
        await asyncio.wait_for(pipeline.run(_job(kb_id, file_id)), WAIT_SECONDS)
    assert raised.value.code is ErrorCode.DEPENDENCY_TIMEOUT
    exceeded = [c for c in log.warning.call_args_list if c.args == ("ingest deadline exceeded",)]
    assert len(exceeded) == 1
    fields = exceeded[0].kwargs
    assert (fields["request_id"], fields["file_id"]) == (REQUEST_ID, file_id)
    assert fields["deadline_seconds"] == DEADLINE_SECONDS and fields["duration_ms"] >= 0
    assert first_batch.is_set()
    assert await _points(qdrant, pool, kb_id, file_id) == []
    row = await file_row(pool, file_id)
    assert (row["status"], row["ingest_lease_until"]) == ("pending", None)  # released for the retry
