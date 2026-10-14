from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from math import isfinite
from time import perf_counter
from typing import Any, Final, Protocol

import litellm
import structlog
from structlog.contextvars import get_contextvars
from tenacity import AsyncRetrying

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.providers.litellm_errors import to_app_error
from thomas_ragx.registry.model import ModelHandle

DURATION_MILLISECONDS: Final = 1000
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
RESULT_INDEX: Final = "index"
RESULT_SCORE: Final = "relevance_score"
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class RerankScore:
    index: int
    score: float


class Reranker(Protocol):
    async def rerank(
        self, model: ModelHandle, query: str, documents: Sequence[str], top_n: int
    ) -> list[RerankScore]: ...


class RerankRecorder(Protocol):
    def observe_rerank_call(self, kind: str, outcome: str, duration_seconds: float) -> None: ...


def _finite_score(value: Any) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    try:
        score = float(value)
    except OverflowError:
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED) from None
    if not isfinite(score):
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    return score


def _validated_scores(results: Any, document_count: int) -> list[RerankScore]:
    if results is None:
        return []
    if not isinstance(results, list):
        raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
    scores = []
    seen: set[int] = set()
    for result in results:
        if not isinstance(result, Mapping):
            raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
        index = result.get(RESULT_INDEX)
        # Provider indices select evidence, so coercion or duplicates could change the cited passage.
        if (
            isinstance(index, bool)
            or not isinstance(index, int)
            or not 0 <= index < document_count
            or index in seen
        ):
            raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)
        seen.add(index)
        scores.append(RerankScore(index, _finite_score(result.get(RESULT_SCORE))))
    return scores


class LiteLLMReranker:
    def __init__(self, retrying_factory: Callable[[], AsyncRetrying], *, metrics: RerankRecorder) -> None:
        self._retrying = retrying_factory
        self._metrics = metrics

    async def _call(
        self, model: ModelHandle, query: str, documents: Sequence[str], top_n: int
    ) -> list[RerankScore]:
        started = perf_counter()
        outcome = OUTCOME_ERROR
        try:
            response = await litellm.arerank(
                model=model.litellm_model,
                query=query,
                documents=list(documents),
                top_n=top_n,
                api_key=model.access.api_key.get_secret_value() if model.access.api_key else None,
                api_base=model.access.base_url or None,
            )
            scores = _validated_scores(response.results, len(documents))
        except Exception as exc:
            raise to_app_error(exc) from None
        else:
            outcome = OUTCOME_SUCCESS
            return scores
        finally:
            duration = perf_counter() - started
            self._metrics.observe_rerank_call(model.access.kind.value, outcome, duration)
            log.info(
                "rerank call completed",
                kind=model.access.kind.value,
                outcome=outcome,
                request_id=get_contextvars().get("request_id", ""),
                duration_ms=duration * DURATION_MILLISECONDS,
            )

    async def rerank(
        self, model: ModelHandle, query: str, documents: Sequence[str], top_n: int
    ) -> list[RerankScore]:
        async for attempt in self._retrying():
            with attempt:
                return await self._call(model, query, documents, top_n)
        raise AssertionError("unreachable: retrying reraises")  # pragma: no cover
