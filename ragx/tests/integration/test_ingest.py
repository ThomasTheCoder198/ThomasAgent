import io
import uuid
from datetime import timedelta
from typing import Any

import pytest
from minio import Minio
from redis.asyncio import Redis

from tests.integration.api_client import api_client, bearer, upload
from tests.integration.ingest_support import _pipeline
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.repository import claim_file
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.worker.consumer import StreamConsumer
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler
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
    pool: Pool, minio_config: dict[str, Any], minio: Minio, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "chinh-sach.md", MARKDOWN)
    await _pipeline(pool, minio, tmp_path).run(_job(kb_id, file_id))
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
    assert list(tmp_path.iterdir()) == []  # the raw copy is always removed


async def test_ingest_refuses_an_object_whose_hash_changed(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis, tmp_path: Any
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
    await consumer.poll_once(IngestHandler(_pipeline(pool, minio, tmp_path)))
    row = await _row(pool, file_id)
    assert (row["status"], row["error_code"]) == ("failed", "INGEST_CONTENT_CHANGED")
    async with pool.connection() as conn:
        count = await (
            await conn.execute(b"SELECT count(*) AS n FROM doc_blocks WHERE file_id = %s", (file_id,))
        ).fetchone()
    assert count is not None and count["n"] == 0


async def test_corrupt_pdf_fails_with_parse_failed(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis, tmp_path: Any
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
    await consumer.poll_once(IngestHandler(_pipeline(pool, minio, tmp_path)))
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
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis, tmp_path: Any
) -> None:
    _kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    pipeline = _pipeline(pool, minio, tmp_path)
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
    pool: Pool, minio_config: dict[str, Any], minio: Minio, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    pipeline = _pipeline(pool, minio, tmp_path)
    await pipeline.run(IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 2, REQUEST_ID))  # wrong version
    assert (await _row(pool, file_id))["status"] == "pending"
    async with api_client(pool, minio_config=minio_config) as (client, _):
        await client.delete(f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=bearer(ORG, "kb:write"))
    await pipeline.run(_job(kb_id, file_id))
    assert (await _row(pool, file_id))["status"] == "pending"


async def test_reingest_replaces_the_previous_outline(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "a.md", MARKDOWN)
    pipeline = _pipeline(pool, minio, tmp_path)
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
    pool: Pool, minio_config: dict[str, Any], minio: Minio, tmp_path: Any
) -> None:
    kb_id, file_id = await _uploaded(pool, minio_config, "table.csv", b"Name,Count\nA,2\nB,3\n")
    await _pipeline(pool, minio, tmp_path).run(_job(kb_id, file_id))
    async with pool.connection() as conn:
        table = await (
            await conn.execute(b"SELECT header, rows FROM kb_tables WHERE file_id = %s", (file_id,))
        ).fetchone()
    assert table is not None
    assert table["header"] == ["Name", "Count"]
    assert table["rows"] == [["A", "2"], ["B", "3"]]
