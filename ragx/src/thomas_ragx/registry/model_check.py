from typing import Any, Final, Protocol

import structlog

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import Capability, ModelRecord, ProviderAccess
from thomas_ragx.registry.model_info import litellm_model_name

PROBE_TEXT: Final = "ping"
PROBE_MAX_TOKENS: Final = 1
log = structlog.get_logger(__name__)


class ModelChecker(Protocol):
    async def check(self, access: ProviderAccess, model: ModelRecord) -> None: ...


class LiteLLMModelChecker:
    """One minimal call per capability; providers like Cohere have no call that needs no model."""

    async def check(self, access: ProviderAccess, model: ModelRecord) -> None:
        import litellm

        target = litellm_model_name(access.kind, model.name)
        options: dict[str, Any] = {
            "api_key": access.api_key.get_secret_value() if access.api_key else None,
            "api_base": access.base_url or None,
        }
        try:
            if Capability.EMBEDDING in model.capabilities:
                await litellm.aembedding(model=target, input=[PROBE_TEXT], **options)
            elif Capability.RERANK in model.capabilities:
                await litellm.arerank(model=target, query=PROBE_TEXT, documents=[PROBE_TEXT], **options)
            elif Capability.CHAT in model.capabilities:
                messages = [{"role": "user", "content": PROBE_TEXT}]
                await litellm.acompletion(
                    model=target, messages=messages, max_tokens=PROBE_MAX_TOKENS, **options
                )
            else:
                raise AppError(  # noqa: TRY301 - this is validation within the provider-call boundary
                    ErrorCode.VALIDATION_FAILED,
                    details={"fields": [{"field": "capabilities", "code": "REQUIRED"}]},
                )
        except AppError:
            raise
        except Exception as exc:
            log.warning("model check failed", kind=access.kind.value, error_type=type(exc).__name__)
            raise AppError(ErrorCode.LLM_PROVIDER_REJECTED) from None
