import httpx
import pytest

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.retry import backoff_seconds, is_retryable, retrying


def fast_config() -> Config:
    return Config(
        minio_access_key="a",
        minio_secret_key="s",
        retry_max_attempts=3,
        retry_base_delay_seconds=0.0,
        retry_max_delay_seconds=0.0,
    )


def test_is_retryable_classifies_errors() -> None:
    assert is_retryable(AppError(ErrorCode.RATE_LIMITED))
    assert not is_retryable(AppError(ErrorCode.NOT_FOUND))
    assert is_retryable(httpx.ConnectError("refused"))


async def test_retrying_stops_after_max_attempts() -> None:
    calls = 0
    with pytest.raises(AppError):
        async for attempt in retrying(fast_config()):
            with attempt:
                calls += 1
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
    assert calls == 3


async def test_retrying_does_not_retry_non_retryable() -> None:
    calls = 0
    with pytest.raises(AppError):
        async for attempt in retrying(fast_config()):
            with attempt:
                calls += 1
                raise AppError(ErrorCode.VALIDATION_FAILED)
    assert calls == 1


@pytest.mark.parametrize("attempt", [1, 2, 3, 10])
def test_backoff_seconds_stays_within_the_capped_exponential_window(attempt: int) -> None:
    base, cap = 0.5, 4.0
    ceiling = min(cap, base * 2 ** (attempt - 1))
    assert all(0 <= backoff_seconds(attempt, base, cap) <= ceiling for _ in range(50))
