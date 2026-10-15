import pytest
from redis.asyncio import Redis
from redis.exceptions import TimeoutError as RedisTimeoutError

from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED
from thomas_ragx.platform.config import Config
from thomas_ragx.resources import REDIS_PROTOCOL, create_redis
from thomas_ragx.worker.consumer import StreamConsumer

pytestmark = pytest.mark.integration
SOCKET_TIMEOUT_SECONDS = 1.0
BLOCK_BELOW_TIMEOUT_MS = 500
BLOCK_ABOVE_TIMEOUT_MS = 1500


async def _noop(*_: object) -> None:
    return None


def _consumer(redis: Redis, block_ms: int) -> StreamConsumer:
    return StreamConsumer(
        redis,
        STREAM_INGEST_REQUESTED,
        "c",
        max_deliveries=3,
        visibility_timeout_ms=60_000,
        block_ms=block_ms,
        batch_size=10,
    )


async def test_an_idle_poll_that_blocks_below_the_socket_timeout_returns_quietly(redis_url: str) -> None:
    config = Config(
        minio_access_key="a",
        minio_secret_key="s",
        redis_url=redis_url,
        redis_socket_timeout_seconds=SOCKET_TIMEOUT_SECONDS,
        stream_block_milliseconds=BLOCK_BELOW_TIMEOUT_MS,
    )
    client = create_redis(config)
    try:
        # The 0.5 s block is also under redis-py's own 5 s default, so this poll alone cannot tell whether the
        # factory passed the configured timeout through: pin the wiring on the very client the consumer uses.
        assert client.connection_pool.connection_kwargs["socket_timeout"] == SOCKET_TIMEOUT_SECONDS
        await client.flushdb()
        consumer = _consumer(client, config.stream_block_milliseconds)
        await consumer.ensure_group()
        await consumer.poll_once(_noop)  # type: ignore[arg-type]
    finally:
        await client.aclose()


async def test_a_block_longer_than_the_socket_timeout_is_the_failure_the_config_check_prevents(
    redis_url: str,
) -> None:
    # Config refuses this pairing, so build the client by hand to pin redis-py's behaviour.
    client = Redis.from_url(
        redis_url, decode_responses=True, protocol=REDIS_PROTOCOL, socket_timeout=SOCKET_TIMEOUT_SECONDS
    )
    try:
        await client.flushdb()
        consumer = _consumer(client, BLOCK_ABOVE_TIMEOUT_MS)
        await consumer.ensure_group()
        with pytest.raises(RedisTimeoutError):
            await consumer.poll_once(_noop)  # type: ignore[arg-type]
    finally:
        await client.aclose()
