from typing import Final

import httpx
from litellm.exceptions import AuthenticationError, BadRequestError, NotFoundError, PermissionDeniedError

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.retry import RETRY_AFTER_HEADER

# Order matters: LiteLLM's classes inherit from each other (BadRequestError is an APIError, …).
REJECTED: Final = (
    AuthenticationError,
    BadRequestError,
    NotFoundError,
    PermissionDeniedError,
)


def to_app_error(exc: Exception) -> AppError:
    """Provider refusals are not retried; everything else (rate limit, timeout, 5xx, network) is (rule 5)."""
    if isinstance(exc, AppError):
        return exc
    if isinstance(exc, REJECTED):
        return AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    response = getattr(exc, "response", None)
    retry_after = response.headers.get(RETRY_AFTER_HEADER) if isinstance(response, httpx.Response) else None
    return AppError(ErrorCode.PROVIDER_UNAVAILABLE, retry_after=retry_after)
