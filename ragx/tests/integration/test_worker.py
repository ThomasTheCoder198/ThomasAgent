import io
import json
import uuid
from datetime import UTC, datetime, timedelta
from typing import Any
from unittest.mock import Mock

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from tests.integration.api_client import api_client, bearer, confirm, post_to_storage, presign, upload
from tests.integration.ingest_support import pipeline_for
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import file_repository
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET, ObjectStorage
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.worker.consumer import CONSUMER_GROUP, DEAD_LETTER_SUFFIX, StreamConsumer, StreamMessage
from thomas_ragx.worker.dlq import DeadLetterReplayer
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler, PurgeHandler
from thomas_ragx.worker.relay import OutboxRelay
from thomas_ragx.worker.sweeper import Sweeper

pytestmark = pytest.mark.integration
ORG = "org-alpha"
REQUEST_ID = "req_8a1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b21"
DEAD_REDIS = "redis://127.0.0.1:1/0"


async def _uploaded_file(pool: Pool, minio_config: dict[str, Any], redis_url: str) -> tuple[str, str]:
    async with api_client(pool, redis_url=redis_url, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        org = await client.put(
            f"/v1/orgs/{ORG}",
            json={"name": "Alpha", "externalParsing": True, "settingsVersion": 1},
            headers=bearer(ORG, "org:provision"),
        )
        kb_id = org.json()["data"]["defaultKbId"]
        uploaded = await upload(client, ORG, kb_id, "w.txt", b"worker", REQUEST_ID)
    assert uploaded.status_code == 200, uploaded.text
    return kb_id, uploaded.json()["data"]["id"]


def _message_fields(payload: dict[str, Any]) -> dict[str, str]:
    return {"payload": json.dumps(payload), "request_id": REQUEST_ID, "org_id": ORG, "outbox_id": "o-1"}


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
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis: Redis,
    redis_url: str,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
) -> None:
    _, file_id = await _uploaded_file(pool, minio_config, redis_url)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = _consumer(redis, STREAM_INGEST_REQUESTED)
    await consumer.ensure_group()
    await consumer.ensure_group()  # idempotent (BUSYGROUP)
    await consumer.poll_once(IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path)))
    pending = await redis.xpending(STREAM_INGEST_REQUESTED, CONSUMER_GROUP)
    assert pending["pending"] == 0
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT status FROM kb_files WHERE id = %s", (file_id,))).fetchone()
    assert row is not None and row["status"] == "ready"


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
        raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)

    for _ in range(3):
        await consumer.poll_once(failing)
    dead = await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)
    assert len(dead) == 1
    fields = dead[0][1]
    assert (fields["request_id"], fields["org_id"], fields["error"]) == (
        REQUEST_ID,
        ORG,
        "PROVIDER_UNAVAILABLE",
    )
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
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis: Redis,
    redis_url: str,
    qdrant: AsyncQdrantClient,
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
    handler = PurgeHandler(pool, ObjectStorage(minio), QdrantStore(qdrant, upsert_batch_size=64))
    await consumer.poll_once(handler)
    assert list(minio.list_objects(RAW_BUCKET, recursive=True)) == []
    assert list(minio.list_objects(PREVIEW_BUCKET, recursive=True)) == []
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT count(*) AS n FROM kb_files")).fetchone()
    assert row is not None and row["n"] == 0
    assert (await redis.xpending(STREAM_KB_PURGE_REQUESTED, CONSUMER_GROUP))["pending"] == 0


async def test_retryable_failure_is_requeued_without_waiting_for_visibility(redis: Redis) -> None:
    await redis.xadd(STREAM_INGEST_REQUESTED, _message_fields({"kb_id": "k"}))
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=3_600_000,
        block_ms=10,
        batch_size=10,
        retry_delay=lambda _: 0.0,
    )
    await consumer.ensure_group()
    seen: list[int] = []

    async def flaky(message: StreamMessage) -> None:
        seen.append(message.deliveries)
        if len(seen) == 1:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)

    await consumer.poll_once(flaky)
    await consumer.poll_once(flaky)  # an hour of visibility, yet the retry runs on the next poll
    assert seen == [1, 2]
    assert (await redis.xpending(STREAM_INGEST_REQUESTED, CONSUMER_GROUP))["pending"] == 0


async def test_non_retryable_error_dead_letters_on_first_delivery(redis: Redis) -> None:
    await redis.xadd(STREAM_INGEST_REQUESTED, _message_fields({"kb_id": str(uuid.uuid4())}))
    hooked: list[tuple[str, str]] = []

    async def hook(message: StreamMessage, code: str) -> None:
        hooked.append((message.org_id, code))

    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=hook,
    )
    await consumer.ensure_group()

    async def corrupt(_: StreamMessage) -> None:
        raise AppError(ErrorCode.INGEST_PARSE_FAILED)

    await consumer.poll_once(corrupt)
    dead = await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)
    assert [entry[1]["error"] for entry in dead] == ["INGEST_PARSE_FAILED"]
    assert hooked == [(ORG, "INGEST_PARSE_FAILED")]


async def test_a_failing_dead_letter_hook_still_acknowledges(redis: Redis) -> None:
    await redis.xadd(STREAM_INGEST_REQUESTED, _message_fields({}))

    async def broken_hook(_: StreamMessage, __: str) -> None:
        raise RuntimeError("db down")

    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=1,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=broken_hook,
    )
    await consumer.ensure_group()

    async def failing(_: StreamMessage) -> None:
        raise AppError(ErrorCode.INGEST_NO_TEXT)

    await consumer.poll_once(failing)
    assert (await redis.xpending(STREAM_INGEST_REQUESTED, CONSUMER_GROUP))["pending"] == 0


async def test_dead_letter_replay_runs_the_job_again(redis: Redis) -> None:
    await redis.xadd(STREAM_INGEST_REQUESTED, _message_fields({"kb_id": "k"}))
    consumer = _consumer(redis, STREAM_INGEST_REQUESTED, max_deliveries=1)
    await consumer.ensure_group()

    async def failing(_: StreamMessage) -> None:
        raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)

    await consumer.poll_once(failing)
    assert len(await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)) == 1
    assert await DeadLetterReplayer(redis).replay(STREAM_INGEST_REQUESTED, None) == 1
    assert await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX) == []
    seen: list[StreamMessage] = []

    async def record(message: StreamMessage) -> None:
        seen.append(message)

    await consumer.poll_once(record)
    assert [(m.request_id, m.org_id, dict(m.payload)) for m in seen] == [(REQUEST_ID, ORG, {"kb_id": "k"})]


async def test_replay_of_named_entries_leaves_the_others(redis: Redis) -> None:
    dlq = STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX
    first = await redis.xadd(dlq, _message_fields({"n": 1}))
    await redis.xadd(dlq, _message_fields({"n": 2}))
    assert await DeadLetterReplayer(redis).replay(STREAM_INGEST_REQUESTED, [first]) == 1
    assert len(await redis.xrange(dlq)) == 1


async def test_ingest_dead_letter_marks_the_file_failed_unless_finished(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis_url: str
) -> None:
    kb_id, file_id = await _uploaded_file(pool, minio_config, redis_url)
    payload = {"kb_id": kb_id, "file_id": file_id, "version": 1}
    message = StreamMessage(STREAM_INGEST_REQUESTED, "1-0", "o", ORG, REQUEST_ID, payload, 3)
    await IngestDeadLetter(pool)(message, "ValueError")  # not a catalog code
    async with pool.connection() as conn:
        row = await (
            await conn.execute(b"SELECT status, error_code FROM kb_files WHERE id = %s", (file_id,))
        ).fetchone()
        assert row is not None and (row["status"], row["error_code"]) == ("failed", "INTERNAL_ERROR")
        await conn.execute(b"UPDATE kb_files SET status = 'ready', error_code = '' WHERE id = %s", (file_id,))
    await IngestDeadLetter(pool)(message, "INGEST_NO_TEXT")
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT status FROM kb_files WHERE id = %s", (file_id,))).fetchone()
    assert row is not None and row["status"] == "ready"


async def test_sweeper_removes_only_expired_slots_and_rowless_objects(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis_url: str
) -> None:
    kb_id, file_id = await _uploaded_file(pool, minio_config, redis_url)  # confirmed: must survive
    async with api_client(pool, redis_url=redis_url, minio_config=minio_config) as (client, _):
        stale = (await presign(client, ORG, kb_id, "stale.txt", 3)).json()["data"]
        fresh = (await presign(client, ORG, kb_id, "fresh.txt", 3)).json()["data"]
    await post_to_storage(stale, b"old", "stale.txt")
    orphan_key = f"{ORG}/{kb_id}/{uuid.uuid4()}"
    minio.put_object(RAW_BUCKET, orphan_key, io.BytesIO(b"x"), 1)
    async with pool.connection() as conn:
        await conn.execute(
            b"UPDATE kb_files SET created_at = now() - interval '2 hours' WHERE id = %s", (stale["fileId"],)
        )
    later = datetime.now(UTC) + timedelta(hours=1)  # every object is older than the cutoff by then
    sweeper = Sweeper(
        pool,
        ObjectStorage(minio),
        cutoff_age=timedelta(minutes=20),
        interval_seconds=1,
        now=lambda: later,
    )
    result = await sweeper.sweep_once()
    keys = {str(item.object_name) for item in minio.list_objects(RAW_BUCKET, recursive=True)}
    assert f"{ORG}/{kb_id}/{file_id}" in keys
    assert orphan_key not in keys and f"{ORG}/{kb_id}/{stale['fileId']}" not in keys
    async with pool.connection() as conn:
        ids = {row["id"] for row in await (await conn.execute(b"SELECT id FROM kb_files")).fetchall()}
    assert uuid.UUID(file_id) in ids and uuid.UUID(fresh["fileId"]) in ids
    assert uuid.UUID(stale["fileId"]) not in ids
    assert (result.slots, result.objects) == (1, 1)


async def test_sweeper_keeps_a_slot_that_a_late_confirm_won(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    redis_url: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    kb_id, _ = await _uploaded_file(pool, minio_config, redis_url)
    async with api_client(pool, redis_url=redis_url, minio_config=minio_config) as (client, _):
        slot = (await presign(client, ORG, kb_id, "late.txt", 4)).json()["data"]
        await post_to_storage(slot, b"late", "late.txt")
        async with pool.connection() as conn:
            await conn.execute(
                b"UPDATE kb_files SET created_at = now() - interval '2 hours' WHERE id = %s",
                (slot["fileId"],),
            )
        listed_slots = file_repository.expired_upload_slots

        async def listed_then_confirmed(conn: Any, older_than_seconds: float) -> Any:
            expired = await listed_slots(conn, older_than_seconds)
            assert [str(s.id) for s in expired] == [slot["fileId"]]
            assert (
                await confirm(client, ORG, kb_id, slot["fileId"])
            ).status_code == 200  # confirm wins the race
            return expired

        monkeypatch.setattr(file_repository, "expired_upload_slots", listed_then_confirmed)
        sweeper = Sweeper(pool, ObjectStorage(minio), cutoff_age=timedelta(minutes=20), interval_seconds=1)
        result = await sweeper.sweep_once()
    keys = {str(item.object_name) for item in minio.list_objects(RAW_BUCKET, recursive=True)}
    assert f"{ORG}/{kb_id}/{slot['fileId']}" in keys
    async with pool.connection() as conn:
        row = await (
            await conn.execute(b"SELECT status FROM kb_files WHERE id = %s", (slot["fileId"],))
        ).fetchone()
    assert row is not None and row["status"] == "pending"
    assert result.slots == 0
