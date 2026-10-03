from datetime import UTC, datetime
from email.utils import parsedate_to_datetime

import httpx
from tenacity import (
    AsyncRetrying,
    RetryCallState,
    retry_if_exception,
    stop_after_attempt,
    wait_random_exponential,
)

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.constants import HTTP_STATUS_RATE_LIMITED, HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.errors import AppError

RETRY_AFTER_HEADER = "Retry-After"


def is_retryable(exc: BaseException) -> bool:
    if isinstance(exc, AppError):
        return exc.retryable()
    if isinstance(exc, httpx.HTTPStatusError):
        return (
            exc.response.status_code == HTTP_STATUS_RATE_LIMITED
            or exc.response.status_code >= HTTP_STATUS_SERVER_ERROR
        )
    return isinstance(exc, httpx.TransportError)


def retrying(config: Config) -> AsyncRetrying:
    backoff = wait_random_exponential(
        multiplier=config.retry_base_delay_seconds, max=config.retry_max_delay_seconds
    )

    def wait(state: RetryCallState) -> float:
        delay = backoff(state)
        exc = state.outcome.exception() if state.outcome else None
        if not isinstance(exc, httpx.HTTPStatusError):
            return delay
        value = exc.response.headers.get(RETRY_AFTER_HEADER)
        if value is None:
            return delay
        try:
            retry_after = float(value)
        except ValueError:
            try:
                retry_after = (parsedate_to_datetime(value) - datetime.now(UTC)).total_seconds()
            except (ValueError, TypeError, OverflowError):
                return delay
        return max(delay, retry_after)

    return AsyncRetrying(
        stop=stop_after_attempt(config.retry_max_attempts),
        wait=wait,
        retry=retry_if_exception(is_retryable),
        reraise=True,
    )
