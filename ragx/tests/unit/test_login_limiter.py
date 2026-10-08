from typing import Any

import pytest
from redis.exceptions import ConnectionError as RedisConnectionError

from thomas_ragx.admin.limiter import LoginLimiter
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError


class _BrokenPipeline:
    async def __aenter__(self) -> "_BrokenPipeline":
        return self

    async def __aexit__(self, *_: object) -> None:
        return None

    def incr(self, _: str) -> None:
        return None

    def expire(self, *_: Any, **__: Any) -> None:
        return None

    async def execute(self) -> None:
        raise RedisConnectionError("redis went away")


class _BrokenRedis:
    def pipeline(self, transaction: bool) -> _BrokenPipeline:
        return _BrokenPipeline()

    async def delete(self, *_: str) -> int:
        raise RedisConnectionError("redis went away")


def _limiter() -> LoginLimiter:
    return LoginLimiter(_BrokenRedis(), max_attempts=5, window_seconds=60)  # type: ignore[arg-type]


async def test_record_failure_fails_closed_when_redis_breaks() -> None:
    # An uncounted failure would give an attacker free attempts, so the login is refused like check() does.
    with pytest.raises(AppError) as refused:
        await _limiter().record_failure("owner@example.com", "127.0.0.1")
    assert refused.value.code is ErrorCode.PROVIDER_UNAVAILABLE


async def test_reset_tolerates_redis_breaking_after_a_correct_password() -> None:
    # The counters expire on their own; a correct password must not turn into a 500.
    await _limiter().reset("owner@example.com", "127.0.0.1")
