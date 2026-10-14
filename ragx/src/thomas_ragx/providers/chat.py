from collections.abc import Callable
from time import perf_counter
from typing import Final, Protocol

import litellm
import structlog
from tenacity import AsyncRetrying

from thomas_ragx.platform.timing import MILLISECONDS_PER_SECOND
from thomas_ragx.providers.litellm_errors import to_app_error
from thomas_ragx.registry.model import ModelHandle

OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
ROLE_USER: Final = "user"
log = structlog.get_logger(__name__)


class ChatModel(Protocol):
    async def complete(self, model: ModelHandle, prompt: str, max_tokens: int) -> str: ...


class ChatRecorder(Protocol):
    def observe_chat_call(self, kind: str, outcome: str, duration_seconds: float) -> None: ...


class LiteLLMChat:
    def __init__(self, retrying_factory: Callable[[], AsyncRetrying], *, metrics: ChatRecorder) -> None:
        self._retrying = retrying_factory
        self._metrics = metrics

    async def _call(self, model: ModelHandle, prompt: str, max_tokens: int) -> str:
        started = perf_counter()
        outcome = OUTCOME_ERROR
        try:
            response = await litellm.acompletion(
                model=model.litellm_model,
                messages=[{"role": ROLE_USER, "content": prompt}],
                max_tokens=max_tokens,
                api_key=model.access.api_key.get_secret_value() if model.access.api_key else None,
                api_base=model.access.base_url or None,
            )
            text = str(response.choices[0].message.content or "").strip()
        except Exception as exc:
            raise to_app_error(exc) from None
        else:
            outcome = OUTCOME_SUCCESS
            return text
        finally:
            duration = perf_counter() - started
            self._metrics.observe_chat_call(model.access.kind.value, outcome, duration)
            log.info(
                "chat call completed",
                kind=model.access.kind.value,
                outcome=outcome,
                duration_ms=duration * MILLISECONDS_PER_SECOND,
            )

    async def complete(self, model: ModelHandle, prompt: str, max_tokens: int) -> str:
        async for attempt in self._retrying():
            with attempt:
                return await self._call(model, prompt, max_tokens)
        raise AssertionError("unreachable: retrying reraises")  # pragma: no cover
