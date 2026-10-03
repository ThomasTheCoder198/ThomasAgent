import httpx
import pytest

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.retry import is_retryable, retrying
from thomas_ragx.platform.settings import Settings


def fast_settings() -> Settings:
    return Settings(
        core_service_token="t",
        minio_access_key="a",
        minio_secret_key="s",
        retry_max_attempts=3,
        retry_base_delay_s=0.0,
        retry_max_delay_s=0.0,
    )


def test_is_retryable() -> None:
    assert is_retryable(AppError(ErrorCode.RATE_LIMITED))
    assert not is_retryable(AppError(ErrorCode.NOT_FOUND))
    assert is_retryable(httpx.ConnectError("refused"))


async def test_retrying_stops_after_max_attempts() -> None:
    calls = 0
    with pytest.raises(AppError):
        async for attempt in retrying(fast_settings()):
            with attempt:
                calls += 1
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
    assert calls == 3


async def test_retrying_does_not_retry_non_retryable() -> None:
    calls = 0
    with pytest.raises(AppError):
        async for attempt in retrying(fast_settings()):
            with attempt:
                calls += 1
                raise AppError(ErrorCode.VALIDATION_FAILED)
    assert calls == 1
