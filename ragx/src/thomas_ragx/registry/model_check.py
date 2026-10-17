from collections.abc import Awaitable, Callable
from time import perf_counter
from typing import Any, Final, Protocol

import structlog

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.providers.openrouter_rerank import rerank as openrouter_rerank
from thomas_ragx.registry.model import Capability, ModelRecord, ProviderAccess, ProviderKind
from thomas_ragx.registry.model_info import litellm_model_name

PROBE_TEXT: Final = "ping"
PROBE_MAX_TOKENS: Final = 1
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
log = structlog.get_logger(__name__)


class ModelChecker(Protocol):
    async def check(self, access: ProviderAccess, model: ModelRecord) -> None: ...

    async def measure_embedding_dims(self, access: ProviderAccess, model: ModelRecord) -> int:
        """Embeds one text and returns the vector length; the embedder passes no `dimensions`, so this is
        the size a KB collection must be created with."""
        ...


def _rejected() -> AppError:
    return AppError(ErrorCode.LLM_PROVIDER_REJECTED)


class LiteLLMModelChecker:
    """One minimal call per capability; providers like Cohere have no call that needs no model."""

    def __init__(self, timeout_seconds: float) -> None:
        self._timeout_seconds = timeout_seconds

    def _options(self, access: ProviderAccess) -> dict[str, Any]:
        return {
            "api_key": access.api_key.get_secret_value() if access.api_key else None,
            "api_base": access.base_url or None,
            "timeout": self._timeout_seconds,
        }

    async def _embed_once(self, access: ProviderAccess, model: ModelRecord) -> int:
        import litellm

        response = await litellm.aembedding(
            model=litellm_model_name(access.kind, model.name), input=[PROBE_TEXT], **self._options(access)
        )
        dims = len(response.data[0]["embedding"])
        if not dims:
            raise _rejected()
        return dims

    async def _probe(self, access: ProviderAccess, model: ModelRecord) -> int | None:
        import litellm

        target = litellm_model_name(access.kind, model.name)
        if Capability.EMBEDDING in model.capabilities:
            return await self._embed_once(access, model)
        if Capability.RERANK in model.capabilities:
            if access.kind is ProviderKind.OPENROUTER:
                await openrouter_rerank(
                    access,
                    model.name,
                    PROBE_TEXT,
                    [PROBE_TEXT],
                    1,
                    timeout_seconds=self._timeout_seconds,
                )
            else:
                await litellm.arerank(
                    model=target, query=PROBE_TEXT, documents=[PROBE_TEXT], **self._options(access)
                )
        elif Capability.CHAT in model.capabilities:
            messages = [{"role": "user", "content": PROBE_TEXT}]
            await litellm.acompletion(
                model=target, messages=messages, max_tokens=PROBE_MAX_TOKENS, **self._options(access)
            )
        else:
            raise AppError(
                ErrorCode.VALIDATION_FAILED,
                details={"fields": [{"field": "capabilities", "code": "REQUIRED"}]},
            )
        return None

    async def _run[T](self, event: str, access: ProviderAccess, call: Callable[[], Awaitable[T]]) -> T:
        """One log event per call; a failure becomes a 4xx code, which the UI does not log again."""
        started = perf_counter()
        outcome = OUTCOME_ERROR
        error_type = ""
        try:
            result = await call()
        except AppError as exc:
            error_type = type(exc).__name__
            raise
        except Exception as exc:
            error_type = type(exc).__name__
            raise _rejected() from None
        else:
            outcome = OUTCOME_SUCCESS
            return result
        finally:
            emit = log.info if outcome == OUTCOME_SUCCESS else log.warning
            emit(
                event,
                kind=access.kind.value,
                outcome=outcome,
                error_type=error_type,
                duration_ms=elapsed_ms(started),
            )

    async def check(self, access: ProviderAccess, model: ModelRecord) -> None:
        await self._run("model check completed", access, lambda: self._probe(access, model))

    async def measure_embedding_dims(self, access: ProviderAccess, model: ModelRecord) -> int:
        return await self._run("embedding probe completed", access, lambda: self._embed_once(access, model))
