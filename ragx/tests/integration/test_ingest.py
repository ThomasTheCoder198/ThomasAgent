import asyncio
import io
import json
import uuid
from collections.abc import Sequence
from datetime import timedelta
from typing import Any
from uuid import UUID

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from tests.integration.api_client import api_client, bearer, upload
from tests.integration.ingest_support import pipeline_for
from tests.rag_fakes import FakeChat, HashEmbedder
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.repository import claim_file
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.registry.model import Capability, ModelHandle, Role
from thomas_ragx.retrieval.qdrant_store import QdrantStore, file_condition, tenant_filter
from thomas_ragx.worker.consumer import StreamConsumer
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler, PurgeHandler
from thomas_ragx.worker.relay import OutboxRelay

pytestmark = pytest.mark.integration
ORG = "org-ingest"
REQUEST_ID = "req_5b1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b99"
MARKDOWN = "# Chính sách\n\nĐoạn mở đầu.\n\n## Đổi trả\n\nTrong 7 ngày.\n".encode()


@pytest.fixture
async def redis(redis_url: str) -> Any:
    client = Redis.from_url(redis_url, decode_responses=True, protocol=2)
    await client.flushdb()
    yield client
    await client.aclose()


@pytest.fixture
def minio(minio_config: dict[str, Any]) -> Minio:
    client = Minio(
        minio_config["endpoint"],
        access_key=minio_config["access_key"],
        secret_key=minio_config["secret_key"],
        secure=False,
    )
    for bucket in (RAW_BUCKET, PREVIEW_BUCKET):
        if not client.bucket_exists(bucket):
            client.make_bucket(bucket)
        for item in client.list_objects(bucket, recursive=True):
            client.remove_object(bucket, str(item.object_name))
    return client


async def _uploaded(pool: Pool, minio_config: dict[str, Any], name: str, content: bytes) -> tuple[str, str]:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        org = await client.put(
            f"/v1/orgs/{ORG}",
            json={"name": "Ingest", "externalParsing": True},
            headers=bearer(ORG, "org:provision"),
        )
        kb_id = org.json()["data"]["defaultKbId"]
        stored = await upload(client, ORG, kb_id, name, content, REQUEST_ID)
    assert stored.status_code == 200, stored.text
    return kb_id, stored.json()["data"]["id"]


def _job(kb_id: str, file_id: str) -> IngestJob:
    return IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 1, REQUEST_ID)


async def _row(pool: Pool, file_id: str) -> dict[str, Any]:
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT * FROM kb_files WHERE id = %s", (file_id,))).fetchone()
    assert row is not None
    return dict(row)


async def test_ingest_parses_a_text_upload_into_blocks_and_marks_it_ready(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "chinh-sach.md", MARKDOWN)
    await pipeline_for(pool, minio, qdrant, tmp_path).run(_job(kb_id, file_id))
    row = await _row(pool, file_id)
    assert (row["status"], row["degraded"], row["parse_provider"], row["error_code"]) == (
        "ready",
        False,
        "local",
        "",
    )
    assert row["ingested_at"] is not None and row["ingest_lease_until"] is None
    async with pool.connection() as conn:
        nodes = await (
            await conn.execute(
                b"SELECT title, depth FROM doc_nodes WHERE file_id = %s ORDER BY position", (file_id,)
            )
        ).fetchall()
        blocks = await (
            await conn.execute(
                b"SELECT block_id, block_type, locator FROM doc_blocks WHERE file_id = %s ORDER BY position",
                (file_id,),
            )
        ).fetchall()
    assert [(n["title"], n["depth"]) for n in nodes] == [
        ("chinh-sach.md", 0),
        ("Chính sách", 1),
        ("Đổi trả", 2),
    ]
    assert [b["block_type"] for b in blocks] == ["heading", "text", "heading", "text"]
    assert blocks[3]["locator"] == {"kind": "text_lines", "lineStart": 7, "lineEnd": 7}
    assert list((tmp_path / "work").iterdir()) == []  # the raw copy is always removed


async def test_ingest_refuses_an_object_whose_hash_changed(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis: Redis,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    swapped = bytes(reversed(MARKDOWN))  # same size, other bytes: what a reused ticket could store
    minio.put_object(RAW_BUCKET, f"{ORG}/{kb_id}/{file_id}", io.BytesIO(swapped), len(swapped))
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )
    await consumer.ensure_group()
    await consumer.poll_once(IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path)))
    row = await _row(pool, file_id)
    assert (row["status"], row["error_code"]) == ("failed", "INGEST_CONTENT_CHANGED")
    async with pool.connection() as conn:
        count = await (
            await conn.execute(b"SELECT count(*) AS n FROM doc_blocks WHERE file_id = %s", (file_id,))
        ).fetchone()
    assert count is not None and count["n"] == 0


async def test_corrupt_pdf_fails_with_parse_failed(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis: Redis,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    _kb_id, file_id = await _uploaded(pool, minio_config, "broken.pdf", b"%PDF-1.7 not really")
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )
    await consumer.ensure_group()
    await consumer.poll_once(IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path)))
    row = await _row(pool, file_id)
    assert (row["status"], row["error_code"]) == ("failed", "INGEST_PARSE_FAILED")
    assert len(await redis.xrange(STREAM_INGEST_REQUESTED + ".dlq")) == 1


async def test_claim_skips_a_leased_file_and_takes_over_an_expired_lease(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    job = _job(kb_id, file_id)
    async with pool.connection() as conn:
        assert await claim_file(conn, job, 600.0) is not None
        assert await claim_file(conn, job, 600.0) is None  # a duplicate delivery while the lease holds
        await conn.execute(
            b"UPDATE kb_files SET ingest_lease_until = now() - %s WHERE id = %s",
            (timedelta(seconds=1), file_id),
        )
        taken = await claim_file(conn, job, 600.0)
    assert taken is not None and taken.collection_name.startswith("kb_")


async def test_a_failed_attempt_releases_its_claim_for_the_retry(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis: Redis,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    _kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    attempts = 0
    parse = pipeline._parser.parse

    async def flaky(*args: Any, **kwargs: Any) -> Any:
        nonlocal attempts
        attempts += 1
        if attempts == 1:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)  # retryable
        return await parse(*args, **kwargs)

    pipeline._parser.parse = flaky  # type: ignore[method-assign]
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )
    await consumer.ensure_group()
    await consumer.poll_once(IngestHandler(pipeline))
    assert (await _row(pool, file_id))["status"] == "pending"  # released, not stuck in "parsing"
    await consumer.poll_once(IngestHandler(pipeline))
    assert (attempts, (await _row(pool, file_id))["status"]) == (2, "ready")


async def test_ingest_skips_a_deleted_or_superseded_job(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    await pipeline.run(IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 2, REQUEST_ID))  # wrong version
    assert (await _row(pool, file_id))["status"] == "pending"
    async with api_client(pool, minio_config=minio_config) as (client, _):
        await client.delete(f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=bearer(ORG, "kb:write"))
    await pipeline.run(_job(kb_id, file_id))
    assert (await _row(pool, file_id))["status"] == "pending"


async def test_reingest_replaces_the_previous_outline(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    await pipeline.run(_job(kb_id, file_id))
    async with pool.connection() as conn:
        await conn.execute(
            b"UPDATE kb_files SET status = 'failed' WHERE id = %s", (file_id,)
        )  # e.g. a DLQ replay
    await pipeline.run(_job(kb_id, file_id))
    async with pool.connection() as conn:
        count = await (
            await conn.execute(b"SELECT count(*) AS n FROM doc_blocks WHERE file_id = %s", (file_id,))
        ).fetchone()
    assert count is not None and count["n"] == 4


async def test_ingest_stores_table_grid(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "table.csv", b"Name,Count\nA,2\nB,3\n")
    await pipeline_for(pool, minio, qdrant, tmp_path).run(_job(kb_id, file_id))
    async with pool.connection() as conn:
        table = await (
            await conn.execute(b"SELECT header, rows FROM kb_tables WHERE file_id = %s", (file_id,))
        ).fetchone()
    assert table is not None
    assert table["header"] == ["Name", "Count"]
    assert table["rows"] == [["A", "2"], ["B", "3"]]


async def _points(qdrant: AsyncQdrantClient, pool: Pool, kb_id: str, file_id: str | None = None) -> list:
    async with pool.connection() as conn:
        row = await (
            await conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,))
        ).fetchone()
    assert row is not None
    if not await qdrant.collection_exists(row["collection_name"]):
        return []
    extra = [file_condition(file_id)] if file_id else []
    found, _ = await qdrant.scroll(
        row["collection_name"], scroll_filter=tenant_filter(ORG, kb_id, *extra), limit=1000
    )
    return found


async def test_ingest_indexes_chunks_with_tenant_payload_and_writes_the_preview(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "chinh-sach.md", MARKDOWN)
    await pipeline_for(pool, minio, qdrant, tmp_path).run(_job(kb_id, file_id))
    row = await _row(pool, file_id)
    points = await _points(qdrant, pool, kb_id, file_id)
    assert row["status"] == "ready" and row["chunk_count"] == len(points) > 0
    assert {p.payload["org_id"] for p in points} == {ORG}
    preview = json.loads(minio.get_object(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json").read())
    assert preview["fileId"] == file_id and preview["blocks"][0]["locator"]["kind"] == "text_lines"


async def test_ingest_uses_the_kbs_model_after_the_role_changes(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    service = create_registry_service(Config(minio_access_key="a", minio_secret_key="s"), pool)
    original = (await service.roles())[Role.EMBEDDING]
    assert original is not None
    other = await service.add_manual_model(
        original.provider_id, "other-embedding", frozenset({Capability.EMBEDDING}), 768, None
    )
    await service.assign_role(Role.EMBEDDING, other.id)
    seen: list[UUID] = []

    async def recording(model_id: UUID) -> ModelHandle:
        seen.append(model_id)
        return await service.model_handle(model_id)

    await pipeline_for(pool, minio, qdrant, tmp_path, models=recording).run(_job(kb_id, file_id))
    assert seen == [original.id]


async def test_dims_mismatch_fails_the_file(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    _kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )
    await consumer.ensure_group()
    short = HashEmbedder(1536, vector_length=1535)
    await consumer.poll_once(IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path, embedder=short)))
    assert ((await _row(pool, file_id))["status"], (await _row(pool, file_id))["error_code"]) == (
        "failed",
        "INGEST_EMBEDDING_MISMATCH",
    )


async def test_points_written_after_a_delete_are_removed(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)

    class DeletingEmbedder(HashEmbedder):
        async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
            async with pool.connection() as conn:  # the operator deletes the file mid-ingest
                await conn.execute(b"UPDATE kb_files SET deleted_at = now() WHERE id = %s", (file_id,))
            return await super().embed(model, texts)

    await pipeline_for(pool, minio, qdrant, tmp_path, embedder=DeletingEmbedder(1536)).run(
        _job(kb_id, file_id)
    )
    assert await _points(qdrant, pool, kb_id, file_id) == []
    assert (await _row(pool, file_id))["status"] != "ready"


async def test_new_version_retires_the_old_one(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    kb_id, first = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    await pipeline.run(_job(kb_id, first))
    async with api_client(pool, minio_config=minio_config) as (client, _):
        second = (await upload(client, ORG, kb_id, "a.md", MARKDOWN + "\nThêm.\n".encode())).json()["data"]
    assert second["version"] == 2
    await pipeline.run(IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(second["id"]), 2, REQUEST_ID))
    assert (await _row(pool, first))["deleted_at"] is not None
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    purge = StreamConsumer(
        redis,
        STREAM_KB_PURGE_REQUESTED,
        "p",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
    )
    await purge.ensure_group()
    await purge.poll_once(PurgeHandler(pool, ObjectStorage(minio), QdrantStore(qdrant, upsert_batch_size=64)))
    assert await _points(qdrant, pool, kb_id, first) == []
    assert await _points(qdrant, pool, kb_id, second["id"]) != []


async def test_kb_purge_removes_its_points(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        await client.put(
            f"/v1/orgs/{ORG}",
            json={"name": "Ingest", "externalParsing": True},
            headers=bearer(ORG, "org:provision"),
        )
        kb_id = (
            await client.post("/v1/knowledge-bases", json={"name": "Tạm"}, headers=bearer(ORG, "kb:write"))
        ).json()["data"]["id"]
        file_id = (await upload(client, ORG, kb_id, "a.md", MARKDOWN)).json()["data"]["id"]
        await pipeline_for(pool, minio, qdrant, tmp_path).run(_job(kb_id, file_id))
        assert await _points(qdrant, pool, kb_id)
        async with pool.connection() as conn:  # read the pointer before the purge removes the KB row
            row = await (
                await conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,))
            ).fetchone()
        assert row is not None
        await client.delete(f"/v1/knowledge-bases/{kb_id}", headers=bearer(ORG, "kb:write"))
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    purge = StreamConsumer(
        redis,
        STREAM_KB_PURGE_REQUESTED,
        "p",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
    )
    await purge.ensure_group()
    await purge.poll_once(PurgeHandler(pool, ObjectStorage(minio), QdrantStore(qdrant, upsert_batch_size=64)))
    remaining, _ = await qdrant.scroll(
        row["collection_name"], scroll_filter=tenant_filter(ORG, kb_id), limit=10
    )
    assert remaining == []


async def test_ingest_writes_section_summaries_when_chat_fast_is_assigned(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    await pipeline_for(pool, minio, qdrant, tmp_path, chat=FakeChat()).run(_job(kb_id, file_id))
    async with pool.connection() as conn:
        rows = await (
            await conn.execute(b"SELECT summary FROM doc_nodes WHERE file_id = %s AND depth > 0", (file_id,))
        ).fetchall()
    assert rows and all(row["summary"].startswith("Tóm tắt") for row in rows)


@pytest.mark.parametrize("pause_at", ["points", "rows"])
async def test_ingest_waits_for_kb_purge_through_point_and_row_deletion(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
    pause_at: str,
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "race.md", MARKDOWN)
    embedded, resume_ingest, purge_paused, resume_purge = (asyncio.Event() for _ in range(4))

    class PausedEmbedder(HashEmbedder):
        async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
            vectors = await super().embed(model, texts)
            embedded.set()
            await resume_ingest.wait()
            return vectors

    class PausedStore(QdrantStore):
        async def delete_kb_points(self, collection: str, org_id: str, kb_id: UUID) -> None:
            await super().delete_kb_points(collection, org_id, kb_id)
            if pause_at == "points":
                purge_paused.set()
                await resume_purge.wait()

    class PausedStorage(ObjectStorage):
        async def remove_prefix(self, bucket: str, prefix: str) -> None:
            await super().remove_prefix(bucket, prefix)
            if pause_at == "rows" and bucket == PREVIEW_BUCKET:
                purge_paused.set()
                await resume_purge.wait()

    pipeline = pipeline_for(pool, minio, qdrant, tmp_path, embedder=PausedEmbedder(1536))
    async with pool.connection() as conn:
        collection = await (
            await conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,))
        ).fetchone()
    assert collection is not None
    await pipeline._indexing.store.ensure_collection(collection["collection_name"], 1536)
    ingestion = asyncio.create_task(pipeline.run(_job(kb_id, file_id)))
    await asyncio.wait_for(embedded.wait(), 5)
    async with pool.connection() as conn:
        row = await (
            await conn.execute(
                b"UPDATE knowledge_bases SET deleted_at = now() WHERE id = %s RETURNING collection_name",
                (kb_id,),
            )
        ).fetchone()
    assert row is not None
    purge = asyncio.create_task(
        PurgeHandler(pool, PausedStorage(minio), PausedStore(qdrant, upsert_batch_size=64))._purge_kb(
            ORG, UUID(kb_id)
        )
    )
    try:
        await asyncio.wait_for(purge_paused.wait(), 5)
        resume_ingest.set()
        with pytest.raises(TimeoutError):
            await asyncio.wait_for(asyncio.shield(ingestion), 3)
    finally:
        resume_ingest.set()
        resume_purge.set()
        await asyncio.wait_for(asyncio.gather(ingestion, purge), 30)
    points, _ = (
        await qdrant.scroll(row["collection_name"], scroll_filter=tenant_filter(ORG, kb_id))
        if (await qdrant.collection_exists(row["collection_name"]))
        else ([], None)
    )
    assert points == []
    assert await ObjectStorage(minio).size_of(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json") is None


async def test_partial_index_failure_serializes_with_file_purge_and_removes_points(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "partial.md", MARKDOWN)
    first_batch, fail_index = asyncio.Event(), asyncio.Event()
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)

    async def partial(collection, owner, points):
        await qdrant.upsert(collection, points[:1], wait=True)
        first_batch.set()
        await fail_index.wait()
        await qdrant.upsert(collection, points[1:], wait=True)
        raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)

    async with pool.connection() as conn:
        collection = await (
            await conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,))
        ).fetchone()
    assert collection is not None
    await pipeline._indexing.store.ensure_collection(collection["collection_name"], 1536)
    pipeline._indexing.store.replace_file_points = partial
    ingestion = asyncio.create_task(pipeline.run(_job(kb_id, file_id)))
    await asyncio.wait_for(first_batch.wait(), 30)
    async with pool.connection() as conn:
        await conn.execute(b"UPDATE kb_files SET deleted_at = now() WHERE id = %s", (file_id,))
    purge = asyncio.create_task(
        PurgeHandler(pool, ObjectStorage(minio), QdrantStore(qdrant, upsert_batch_size=64))._purge_file(
            ORG, UUID(kb_id), UUID(file_id)
        )
    )
    try:
        with pytest.raises(TimeoutError):
            await asyncio.wait_for(asyncio.shield(purge), 3)
    finally:
        fail_index.set()
        results = await asyncio.wait_for(asyncio.gather(ingestion, purge, return_exceptions=True), 30)
    assert isinstance(results[0], AppError)
    assert results[1] is None
    assert await _points(qdrant, pool, kb_id, file_id) == []
    await pipeline.run(_job(kb_id, file_id))  # a retry skips the removed file; cleanup already completed
    assert await _points(qdrant, pool, kb_id, file_id) == []


async def test_delete_during_preview_removes_points_and_the_recreated_preview(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "preview-race.md", MARKDOWN)

    class DeletingStorage(ObjectStorage):
        async def put_bytes(self, bucket: str, key: str, data: bytes, content_type: str) -> None:
            async with pool.connection() as conn:
                await conn.execute(b"UPDATE kb_files SET deleted_at = now() WHERE id = %s", (file_id,))
            await super().put_bytes(bucket, key, data, content_type)

    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    pipeline._storage = DeletingStorage(minio)
    await pipeline.run(_job(kb_id, file_id))
    assert await _points(qdrant, pool, kb_id, file_id) == []
    assert await ObjectStorage(minio).size_of(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json") is None
    assert (await _row(pool, file_id))["status"] != "ready"
