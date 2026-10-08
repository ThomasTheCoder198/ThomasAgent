import asyncio
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Final

import structlog
from redis.asyncio import Redis

from thomas_ragx.db.pool import Pool, create_pool
from thomas_ragx.kb.storage import ObjectStorage, create_storage
from thomas_ragx.platform.config import Config

CHECK_POSTGRES: Final = "postgres"
CHECK_REDIS: Final = "redis"
CHECK_STORAGE: Final = "storage"
# Pinned: the stream consumer reads RESP2 reply shapes (lists of [id, fields] pairs)
# whatever redis-py's default is.
REDIS_PROTOCOL: Final = 2
log = structlog.get_logger(__name__)


@dataclass
class Resources:
    pool: Pool
    redis: Redis
    storage: ObjectStorage
    connect_timeout_seconds: float
    readiness_timeout_seconds: float

    @classmethod
    def from_config(cls, config: Config) -> "Resources":
        return cls(
            pool=create_pool(
                config.database_url.get_secret_value(),
                config.database_pool_min_size,
                config.database_pool_max_size,
            ),
            redis=Redis.from_url(config.redis_url, decode_responses=True, protocol=REDIS_PROTOCOL),
            storage=create_storage(config),
            connect_timeout_seconds=config.database_connect_timeout_seconds,
            readiness_timeout_seconds=config.readiness_timeout_seconds,
        )

    async def open(self) -> None:
        await self.pool.open(wait=True, timeout=self.connect_timeout_seconds)

    async def close(self) -> None:
        await self.pool.close()
        await self.redis.aclose()

    async def _postgres(self) -> bool:
        async with self.pool.connection() as conn:
            await conn.execute(b"SELECT 1")
        return True

    async def _redis(self) -> bool:
        return bool(await self.redis.ping())

    async def _check(self, name: str, probe: Callable[[], Awaitable[bool]]) -> bool:
        try:
            return await asyncio.wait_for(probe(), self.readiness_timeout_seconds)
        except Exception as exc:
            log.warning("readiness check failed", check=name, error_type=type(exc).__name__)
            return False

    async def readiness(self) -> dict[str, bool]:
        probes = {CHECK_POSTGRES: self._postgres, CHECK_REDIS: self._redis, CHECK_STORAGE: self.storage.ready}
        results = await asyncio.gather(*(self._check(name, probe) for name, probe in probes.items()))
        return dict(zip(probes, results, strict=True))
