from collections.abc import Sequence
from typing import Any

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from tests.integration.ingest_support import MARKDOWN, file_row, pipeline_for, uploaded_file
from tests.rag_fakes import HashEmbedder
from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import ModelHandle
from thomas_ragx.worker.consumer import DEAD_LETTER_SUFFIX, FIELD_DELIVERIES, FIELD_ERROR, StreamConsumer
from thomas_ragx.worker.dlq import DeadLetterReplayer
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler
from thomas_ragx.worker.relay import OutboxRelay

pytestmark = pytest.mark.integration
DIMS = 1536
MAX_DELIVERIES = 3


class FlakyEmbedder(HashEmbedder):
    """Provider outage until `healthy` flips: the retryable path to the DLQ."""

    def __init__(self, dims: int) -> None:
        super().__init__(dims)
        self.healthy = False
        self.failed_calls = 0

    async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
        if not self.healthy:
            self.failed_calls += 1
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
        return await super().embed(model, texts)


def _consumer(redis: Redis, pool: Pool) -> StreamConsumer:
    return StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=MAX_DELIVERIES,
        visibility_timeout_ms=0,
        block_ms=10,
        batch_size=10,
        on_dead_letter=IngestDeadLetter(pool),
    )


async def _drain(consumer: StreamConsumer, handler: IngestHandler, rounds: int) -> None:
    for _ in range(rounds):
        await consumer.poll_once(handler)


async def test_a_provider_outage_dead_letters_and_a_replay_finishes_the_file(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    _kb_id, file_id = await uploaded_file(pool, minio_config, "a.md", MARKDOWN)
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    embedder = FlakyEmbedder(DIMS)
    handler = IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path, embedder=embedder))
    consumer = _consumer(redis, pool)
    await consumer.ensure_group()
    await _drain(consumer, handler, MAX_DELIVERIES - 1)
    # A retryable failure must not be dead-lettered before its last delivery.
    assert (await file_row(pool, file_id))["status"] != "failed"
    assert await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX) == []
    await _drain(consumer, handler, 1)
    row = await file_row(pool, file_id)
    assert (row["status"], row["error_code"]) == ("failed", "PROVIDER_UNAVAILABLE")
    [(_, dead)] = await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)
    assert (dead[FIELD_ERROR], dead[FIELD_DELIVERIES]) == ("PROVIDER_UNAVAILABLE", str(MAX_DELIVERIES))
    assert embedder.failed_calls == MAX_DELIVERIES
    embedder.healthy = True
    assert await DeadLetterReplayer(redis).replay(STREAM_INGEST_REQUESTED, None) == 1
    await consumer.poll_once(handler)
    row = await file_row(pool, file_id)
    assert row["status"] == "ready" and row["chunk_count"] > 0 and row["error_code"] == ""
    assert await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX) == []


async def test_a_corrupt_file_replays_into_the_same_failure(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    redis: Redis,
    tmp_path: Any,
) -> None:
    _kb_id, file_id = await uploaded_file(pool, minio_config, "broken.pdf", b"%PDF-1.7 not really")
    await OutboxRelay(pool, redis, batch_size=10, poll_interval_seconds=0.01).publish_batch()
    handler = IngestHandler(pipeline_for(pool, minio, qdrant, tmp_path))
    consumer = _consumer(redis, pool)
    await consumer.ensure_group()
    await consumer.poll_once(handler)
    assert await DeadLetterReplayer(redis).replay(STREAM_INGEST_REQUESTED, None) == 1
    await consumer.poll_once(handler)
    row = await file_row(pool, file_id)
    assert (row["status"], row["error_code"]) == ("failed", "INGEST_PARSE_FAILED")
    assert len(await redis.xrange(STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX)) == 1
