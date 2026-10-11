import io
import json
import uuid
from typing import Any
from unittest.mock import Mock

import pytest
from minio import Minio
from redis.asyncio import Redis

from tests.integration.api_client import api_client, bearer, upload
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.worker.consumer import CONSUMER_GROUP, DEAD_LETTER_SUFFIX, StreamConsumer, StreamMessage
from thomas_ragx.worker.handlers import IngestHandler, PurgeHandler
from thomas_ragx.worker.relay import OutboxRelay

pytestmark = pytest.mark.integration
ORG = "org-alpha"
REQUEST_ID = "req_8a1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b21"
DEAD_REDIS = "redis://127.0.0.1:1/0"


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


async def _uploaded_file(pool: Pool, minio_config: dict[str, Any], redis_url: str) -> tuple[str, str]:
    async with api_client(pool, redis_url=redis_url, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        org = await client.put(
            f"/v1/orgs/{ORG}",
            json={"name": "Alpha", "externalParsing": True},
            headers=bearer(ORG, "org:provision"),
        )
        kb_id = org.json()["data"]["defaultKbId"]
        uploaded = await upload(client, ORG, kb_id, "w.txt", b"worker", REQUEST_ID)
    assert uploaded.status_code == 200, uploaded.text
    return kb_id, uploaded.json()["data"]["id"]


def _consumer(redis: Redis, stream: str, max_deliveries: int = 3) -> StreamConsumer:
    return StreamConsumer(
        redis,
        stream,
        "test-consumer",
        max_deliveries=max_deliveries,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
    )


async def test_relay_publishes_after_redis_recovers(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis
) -> None:
    # The upload itself never touches Redis: it succeeds while Redis is unreachable.
    await _uploaded_file(pool, minio_config, DEAD_REDIS)
    recorder = Mock()
    dead = Redis.from_url(DEAD_REDIS, decode_responses=True, protocol=2)
    with pytest.raises(Exception):  # noqa: B017 - any connection error
        await OutboxRelay(
            pool, dead, batch_size=10, poll_interval_seconds=0.01, metrics=recorder
        ).publish_batch()
    await dead.aclose()
    async with pool.connection() as conn:
        pending = await (
            await conn.execute(b"SELECT count(*) AS n FROM outbox WHERE published_at IS NULL")
        ).fetchone()
    assert pending is not None and pending["n"] == 1
    assert (
        await OutboxRelay(
            pool, redis, batch_size=10, poll_interval_seconds=0.01, metrics=recorder
        ).publish_batch()
        == 1
    )
    entries = await redis.xrange(STREAM_INGEST_REQUESTED)
    assert len(entries) == 1
    fields = entries[0][1]
    assert fields["request_id"] == REQUEST_ID
    assert fields["org_id"] == ORG
    assert await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch() == 0
    assert [call.args[0] for call in recorder.observe_relay.call_args_list] == ["error", "success"]
    assert all(call.args[1] >= 0 for call in recorder.observe_relay.call_args_list)


async def test_ingest_handler_acks_a_received_file(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis, redis_url: str
) -> None:
    await _uploaded_file(pool, minio_config, redis_url)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = _consumer(redis, STREAM_INGEST_REQUESTED)
    await consumer.ensure_group()
    await consumer.ensure_group()  # idempotent (BUSYGROUP)
    await consumer.poll_once(IngestHandler(pool, ObjectStorage(minio)))
    pending = await redis.xpending(STREAM_INGEST_REQUESTED, CONSUMER_GROUP)
    assert pending["pending"] == 0


async def test_failing_job_moves_to_the_dlq_with_its_request_id(redis: Redis) -> None:
    await redis.xadd(
        STREAM_INGEST_REQUESTED,
        {
            "payload": json.dumps({"kb_id": str(uuid.uuid4())}),
            "request_id": REQUEST_ID,
            "org_id": ORG,
            "outbox_id": "o-1",
        },
    )
    recorder = Mock()
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "test-consumer",
        max_deliveries=2,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        metrics=recorder,
    )
    await consumer.ensure_group()
    seen: list[StreamMessage] = []

    async def failing(message: StreamMessage) -> None:
        seen.append(message)
        raise AppError(ErrorCode.KB_FILE_NOT_FOUND)

    for _ in range(3):
        await consumer.poll_once(failing)
    dead = await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)
    assert len(dead) == 1
    fields = dead[0][1]
    assert (fields["request_id"], fields["org_id"], fields["error"]) == (REQUEST_ID, ORG, "KB_FILE_NOT_FOUND")
    assert [m.deliveries for m in seen] == [1, 2]
    assert [call.args[1] for call in recorder.observe_job.call_args_list] == ["retry", "dead_letter"]
    assert all(call.args[0] == STREAM_INGEST_REQUESTED for call in recorder.observe_job.call_args_list)
    assert (await redis.xpending(STREAM_INGEST_REQUESTED, CONSUMER_GROUP))["pending"] == 0


async def test_forged_request_id_is_replaced_with_a_job_id(redis: Redis) -> None:
    await redis.xadd(
        STREAM_INGEST_REQUESTED, {"payload": "{}", "request_id": "forged id", "org_id": ORG, "outbox_id": "o"}
    )
    consumer = _consumer(redis, STREAM_INGEST_REQUESTED)
    await consumer.ensure_group()
    seen: list[StreamMessage] = []

    async def record(message: StreamMessage) -> None:
        seen.append(message)

    await consumer.poll_once(record)
    assert seen[0].request_id.startswith("job_")


async def test_purge_handler_removes_objects_and_rows(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis, redis_url: str
) -> None:
    kb_id, file_id = await _uploaded_file(pool, minio_config, redis_url)
    minio.put_object(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json", io.BytesIO(b"{}"), 2)
    async with api_client(pool, redis_url=redis_url, minio_config=minio_config) as (client, _):
        deleted = await client.delete(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=bearer(ORG, "kb:write")
        )
    assert deleted.status_code == 204
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = _consumer(redis, STREAM_KB_PURGE_REQUESTED)
    await consumer.ensure_group()
    handler = PurgeHandler(pool, ObjectStorage(minio))
    await consumer.poll_once(handler)
    assert list(minio.list_objects(RAW_BUCKET, recursive=True)) == []
    assert list(minio.list_objects(PREVIEW_BUCKET, recursive=True)) == []
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT count(*) AS n FROM kb_files")).fetchone()
    assert row is not None and row["n"] == 0
    assert (await redis.xpending(STREAM_KB_PURGE_REQUESTED, CONSUMER_GROUP))["pending"] == 0
