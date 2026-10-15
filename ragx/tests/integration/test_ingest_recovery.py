"""An in-progress file whose attempt vanished (crash, skipped delivery) is recovered (fix round 1, m1/m3)."""

import json
import uuid
from datetime import timedelta
from typing import Any
from unittest.mock import Mock

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from tests.integration.ingest_support import MARKDOWN, ORG, REQUEST_ID, file_row, pipeline_for, uploaded_file
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest import repository as ingest_repository
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.repository import claim_file
from thomas_ragx.kb import file_repository
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.worker import recovery as recovery_module
from thomas_ragx.worker.consumer import DEAD_LETTER_SUFFIX, FIELD_ERROR, StreamConsumer
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler
from thomas_ragx.worker.recovery import IngestRecovery
from thomas_ragx.worker.relay import FIELD_PAYLOAD, OutboxRelay
from thomas_ragx.worker.sweeper import Sweeper

pytestmark = pytest.mark.integration
GRACE_SECONDS = 60.0
MAX_RECOVERIES = 3
LEASE_SECONDS = 600.0


def _job(kb_id: str, file_id: str) -> IngestJob:
    return IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 1, REQUEST_ID)


def _sweeper(pool: Pool, minio: Minio, redis: Redis) -> Sweeper:
    return Sweeper(
        pool,
        ObjectStorage(minio),
        cutoff_age=timedelta(minutes=20),
        interval_seconds=1,
        recovery=IngestRecovery(pool, redis, grace_seconds=GRACE_SECONDS, max_recoveries=MAX_RECOVERIES),
    )


async def _publish_and_ack_original(pool: Pool) -> None:
    """The confirm's message was delivered and acknowledged: no stream entry is left for this file."""
    async with pool.connection() as conn:
        await conn.execute(b"UPDATE outbox SET published_at = now() WHERE published_at IS NULL")


async def _lease_ended(pool: Pool, file_id: str, seconds_ago: float) -> None:
    async with pool.connection() as conn:
        await conn.execute(
            b"UPDATE kb_files SET ingest_lease_until = now() - make_interval(secs => %s) WHERE id = %s",
            (seconds_ago, file_id),
        )


async def _ingest_outbox_payloads(pool: Pool) -> list[dict[str, Any]]:
    async with pool.connection() as conn:
        rows = await (
            await conn.execute(
                b"SELECT payload FROM outbox WHERE stream = %s AND published_at IS NULL",
                (STREAM_INGEST_REQUESTED,),
            )
        ).fetchall()
    return [row["payload"] for row in rows]


async def test_a_crashed_attempt_whose_redelivery_was_skipped_is_recovered_to_ready(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "crash.md", MARKDOWN)
    await _publish_and_ack_original(pool)
    async with pool.connection() as conn:
        crashed = await claim_file(
            conn, _job(kb_id, file_id), LEASE_SECONDS
        )  # A claims, then its worker dies
    assert crashed is not None
    pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
    await pipeline.run(_job(kb_id, file_id))  # m3: a redelivery meets the live lease, skips, and is acked
    assert (await file_row(pool, file_id))["status"] == "parsing"
    sweeper = _sweeper(pool, minio, redis)
    await _lease_ended(pool, file_id, GRACE_SECONDS / 2)  # Redis reclaim still has its chance first
    assert (await sweeper.sweep_once()).requeued == 0
    await _lease_ended(pool, file_id, GRACE_SECONDS + 1)
    result = await sweeper.sweep_once()
    assert (result.requeued, result.failed) == (1, 0)
    row = await file_row(pool, file_id)
    assert (row["status"], row["ingest_lease_until"], row["ingest_attempt_id"]) == ("pending", None, None)
    assert row["ingest_recoveries"] == 1
    assert await _ingest_outbox_payloads(pool) == [{"kb_id": kb_id, "file_id": file_id, "version": 1}]
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    consumer = StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=MAX_RECOVERIES,
        visibility_timeout_ms=60_000,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )
    await consumer.ensure_group()
    await consumer.poll_once(IngestHandler(pipeline))
    assert (await file_row(pool, file_id))["status"] == "ready"


async def test_a_file_that_keeps_vanishing_ends_failed_in_the_dlq(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "poison.md", MARKDOWN)
    await _publish_and_ack_original(pool)
    async with pool.connection() as conn:
        assert await claim_file(conn, _job(kb_id, file_id), LEASE_SECONDS) is not None
        await conn.execute(
            b"UPDATE kb_files SET ingest_recoveries = %s WHERE id = %s", (MAX_RECOVERIES, file_id)
        )
    await _lease_ended(pool, file_id, GRACE_SECONDS + 1)
    result = await _sweeper(pool, minio, redis).sweep_once()
    assert (result.requeued, result.failed) == (0, 1)
    row = await file_row(pool, file_id)
    assert (row["status"], row["error_code"], row["ingest_lease_until"]) == ("failed", "INTERNAL_ERROR", None)
    assert await _ingest_outbox_payloads(pool) == []
    dead = await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)
    assert len(dead) == 1
    _, fields = dead[0]
    assert json.loads(fields[FIELD_PAYLOAD]) == {"kb_id": kb_id, "file_id": file_id, "version": 1}
    assert fields[FIELD_ERROR] == "INTERNAL_ERROR"
    async with pool.connection() as conn:  # the owner's explicit retry starts a fresh recovery budget
        assert (
            await file_repository.requeue_failed(conn, ORG, uuid.UUID(kb_id), uuid.UUID(file_id)) is not None
        )
    assert (await file_row(pool, file_id))["ingest_recoveries"] == 0


async def test_recovery_leaves_live_leases_and_settled_files_alone(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis
) -> None:
    kb_id, live_id = await uploaded_file(pool, minio_config, "live.md", MARKDOWN)
    _, deleted_id = await uploaded_file(pool, minio_config, "deleted.md", MARKDOWN + b"\nThem.\n")
    await _publish_and_ack_original(pool)
    async with pool.connection() as conn:
        assert await claim_file(conn, _job(kb_id, live_id), LEASE_SECONDS) is not None
        assert await claim_file(conn, _job(kb_id, deleted_id), LEASE_SECONDS) is not None
        await conn.execute(b"UPDATE kb_files SET deleted_at = now() WHERE id = %s", (deleted_id,))
    await _lease_ended(pool, deleted_id, GRACE_SECONDS + 1)
    result = await _sweeper(pool, minio, redis).sweep_once()
    assert (result.requeued, result.failed) == (0, 0)
    assert (await file_row(pool, live_id))["status"] == "parsing"
    assert await _ingest_outbox_payloads(pool) == []


class _RedisDown:
    async def xadd(self, *_: Any, **__: Any) -> None:
        raise ConnectionError("redis down")


async def test_a_redis_outage_neither_stops_other_dead_letters_nor_the_sweep(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, monkeypatch: pytest.MonkeyPatch
) -> None:
    kb_id, first = await uploaded_file(pool, minio_config, "poison-1.md", MARKDOWN)
    _, second = await uploaded_file(pool, minio_config, "poison-2.md", MARKDOWN + b"\nHai.\n")
    await _publish_and_ack_original(pool)
    async with pool.connection() as conn:
        for file_id in (first, second):
            assert await claim_file(conn, _job(kb_id, file_id), LEASE_SECONDS) is not None
        await conn.execute(b"UPDATE kb_files SET ingest_recoveries = %s", (MAX_RECOVERIES,))
    for file_id in (first, second):
        await _lease_ended(pool, file_id, GRACE_SECONDS + 1)
    log = Mock()
    monkeypatch.setattr(recovery_module, "log", log)
    recovery = IngestRecovery(pool, _RedisDown(), grace_seconds=GRACE_SECONDS, max_recoveries=MAX_RECOVERIES)  # type: ignore[arg-type]
    sweeper = Sweeper(
        pool, ObjectStorage(minio), cutoff_age=timedelta(minutes=20), interval_seconds=1, recovery=recovery
    )
    result = await sweeper.sweep_once()
    assert result.failed == 2
    assert {(await file_row(pool, f))["status"] for f in (first, second)} == {"failed"}
    events = [c.args[0] for c in log.error.call_args_list]
    assert (
        events.count("stalled ingest dead letter failed") == 2 and events.count("stalled ingest failed") == 2
    )


async def test_a_recovery_outage_does_not_cost_the_slot_and_object_sweeps(
    pool: Pool, minio: Minio, redis: Redis, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def broken(*_: Any) -> None:
        raise ConnectionError("database down")

    monkeypatch.setattr(ingest_repository, "requeue_stalled", broken)
    slots, objects = Mock(return_value=None), Mock(return_value=None)
    sweeper = _sweeper(pool, minio, redis)

    async def counted_slots() -> int:
        slots()
        return 0

    async def counted_objects() -> int:
        objects()
        return 0

    monkeypatch.setattr(sweeper, "_sweep_slots", counted_slots)
    monkeypatch.setattr(sweeper, "_sweep_objects", counted_objects)
    result = await sweeper.sweep_once()
    assert (result.requeued, result.failed) == (0, 0)
    slots.assert_called_once()
    objects.assert_called_once()


async def test_stalled_files_of_a_tombstoned_kb_wait_for_its_purge(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, redis: Redis
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "tombstone.md", MARKDOWN)
    await _publish_and_ack_original(pool)
    async with pool.connection() as conn:
        assert await claim_file(conn, _job(kb_id, file_id), LEASE_SECONDS) is not None
        await conn.execute(b"UPDATE knowledge_bases SET deleted_at = now() WHERE id = %s", (kb_id,))
    await _lease_ended(pool, file_id, GRACE_SECONDS + 1)
    result = await _sweeper(pool, minio, redis).sweep_once()
    assert (result.requeued, result.failed) == (0, 0)
    assert (await file_row(pool, file_id))["status"] == "parsing"
    assert await redis.xlen(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX) == 0


async def test_the_recovery_budget_resets_on_a_claim_from_failed_and_on_finish(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Any
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "budget.md", MARKDOWN)
    async with pool.connection() as conn:  # e.g. a DLQ replay of a sweep-failed file
        await conn.execute(
            b"UPDATE kb_files SET status = 'failed', ingest_recoveries = %s WHERE id = %s",
            (MAX_RECOVERIES, file_id),
        )
        assert await claim_file(conn, _job(kb_id, file_id), LEASE_SECONDS) is not None
    assert (await file_row(pool, file_id))["ingest_recoveries"] == 0
    async with pool.connection() as conn:  # a recovered file that then finishes
        await conn.execute(
            b"UPDATE kb_files SET status = 'pending', ingest_lease_until = NULL, ingest_recoveries = 2 "
            b"WHERE id = %s",
            (file_id,),
        )
    await pipeline_for(pool, minio, qdrant, tmp_path).run(_job(kb_id, file_id))
    row = await file_row(pool, file_id)
    assert (row["status"], row["ingest_recoveries"]) == ("ready", 0)
