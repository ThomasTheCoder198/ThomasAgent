import hashlib
from typing import Final

import structlog
from redis.asyncio import Redis
from redis.exceptions import RedisError

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

KEY_PREFIX: Final = "ragx:login:"
log = structlog.get_logger(__name__)


def _keys(email: str, ip: str) -> tuple[str, str]:
    # The email is hashed so Redis never holds the operator's address in clear text.
    return KEY_PREFIX + "email:" + hashlib.sha256(email.encode()).hexdigest(), KEY_PREFIX + "ip:" + ip


class LoginLimiter:
    """Fails closed: without Redis there is no attempt budget, so login is refused rather than unlimited."""

    def __init__(self, redis: Redis, max_attempts: int, window_seconds: int) -> None:
        self._redis = redis
        self._max_attempts = max_attempts
        self._window = window_seconds

    async def check(self, email: str, ip: str) -> None:
        try:
            counts = await self._redis.mget(list(_keys(email, ip)))
        except RedisError as exc:
            log.error("login limiter unavailable", error_type=type(exc).__name__)  # noqa: TRY400
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from None
        if any(int(count or 0) >= self._max_attempts for count in counts):
            raise AppError(ErrorCode.RATE_LIMITED)

    async def record_failure(self, email: str, ip: str) -> None:
        async with self._redis.pipeline(transaction=True) as pipe:
            for key in _keys(email, ip):
                pipe.incr(key)
                pipe.expire(key, self._window, nx=True)
            await pipe.execute()

    async def reset(self, email: str, ip: str) -> None:
        await self._redis.delete(*_keys(email, ip))
