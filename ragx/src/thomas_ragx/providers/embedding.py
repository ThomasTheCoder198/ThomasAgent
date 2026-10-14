from collections.abc import Callable, Sequence
from time import perf_counter
from typing import Any, Final, Protocol

import litellm
import structlog
from tenacity import AsyncRetrying

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.providers.litellm_errors import to_app_error
from thomas_ragx.registry.model import ModelHandle

DURATION_MILLISECONDS: Final = 1000
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
RESPONSE_INDEX: Final = "index"
RESPONSE_EMBEDDING: Final = "embedding"
log = structlog.get_logger(__name__)


class Embedder(Protocol):
    async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]: ...


class EmbeddingRecorder(Protocol):
    def observe_embedding_call(self, kind: str, outcome: str, duration_seconds: float) -> None: ...


def _ordered_vectors(data: Any, count: int) -> list[list[float]]:
    items = sorted(data, key=lambda item: item[RESPONSE_INDEX])
    if [item[RESPONSE_INDEX] for item in items] != list(range(count)):
        raise AppError(ErrorCode.INGEST_EMBEDDING_MISMATCH)
    return [list(item[RESPONSE_EMBEDDING]) for item in items]


class LiteLLMEmbedder:
    """Call the module attribute so OpenInference sees every embedding request."""

    def __init__(self, retrying_factory: Callable[[], AsyncRetrying], *, metrics: EmbeddingRecorder) -> None:
        self._retrying = retrying_factory
        self._metrics = metrics

    async def _call(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
        started = perf_counter()
        outcome = OUTCOME_ERROR
        try:
            response = await litellm.aembedding(
                model=model.litellm_model,
                input=list(texts),
                api_key=model.access.api_key.get_secret_value() if model.access.api_key else None,
                api_base=model.access.base_url or None,
            )
            vectors = _ordered_vectors(response.data, len(texts))
        except Exception as exc:
            raise to_app_error(exc) from None
        else:
            outcome = OUTCOME_SUCCESS
            return vectors
        finally:
            duration = perf_counter() - started
            attributes = {"kind": model.access.kind.value, "outcome": outcome}
            self._metrics.observe_embedding_call(model.access.kind.value, outcome, duration)
            log.info("embedding call completed", **attributes, duration_ms=duration * DURATION_MILLISECONDS)

    async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
        async for attempt in self._retrying():
            with attempt:
                return await self._call(model, texts)
        raise AssertionError("unreachable: retrying reraises")  # pragma: no cover
