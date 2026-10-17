from collections.abc import Mapping, Sequence
from typing import Any, Final

import httpx

from thomas_ragx.platform.constants import HTTP_STATUS_RATE_LIMITED, HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import ProviderAccess

RERANK_URL: Final = "https://openrouter.ai/api/v1/rerank"
AUTHORIZATION_HEADER: Final = "Authorization"
RETRY_AFTER_HEADER: Final = "Retry-After"


async def rerank(
    access: ProviderAccess,
    model_name: str,
    query: str,
    documents: Sequence[str],
    top_n: int,
    *,
    timeout_seconds: float,
) -> Any:
    """OpenRouter has a rerank endpoint, but LiteLLM does not dispatch rerank to it."""
    if access.api_key is None:
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    async with httpx.AsyncClient(timeout=timeout_seconds) as client:
        try:
            response = await client.post(
                RERANK_URL,
                headers={AUTHORIZATION_HEADER: f"Bearer {access.api_key.get_secret_value()}"},
                json={"model": model_name, "query": query, "documents": list(documents), "top_n": top_n},
            )
            response.raise_for_status()
        except httpx.HTTPStatusError as exc:
            code = (
                ErrorCode.PROVIDER_UNAVAILABLE
                if exc.response.status_code == HTTP_STATUS_RATE_LIMITED
                or exc.response.status_code >= HTTP_STATUS_SERVER_ERROR
                else ErrorCode.LLM_PROVIDER_REJECTED
            )
            raise AppError(code, retry_after=exc.response.headers.get(RETRY_AFTER_HEADER)) from None
        except httpx.TransportError:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from None
    try:
        payload = response.json()
    except ValueError:
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED) from None
    if not isinstance(payload, Mapping) or "results" not in payload:
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    return payload["results"]
