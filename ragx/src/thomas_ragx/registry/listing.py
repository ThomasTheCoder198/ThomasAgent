import asyncio
from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Protocol

import structlog
from openai import AsyncOpenAI

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import ProviderAccess, ProviderKind

# openai's SDK refuses an empty key; keyless local servers (Ollama, vLLM) ignore this value.
KEYLESS_PLACEHOLDER: Final = "keyless"
log = structlog.get_logger(__name__)


class ListingMethod(StrEnum):
    LITELLM = "litellm"
    OPENAI_SDK = "openai_sdk"
    MANUAL = "manual"


@dataclass(frozen=True)
class ListingRoute:
    method: ListingMethod
    litellm_provider: str = ""
    default_base_url: str = ""
    requires_api_key: bool = True


# The M1.1 probe of LiteLLM 1.101.2 decided these rows.
# Its OpenAI lister drops the base-URL path, so these servers use the official openai SDK.
LISTING_ROUTES: Final[Mapping[ProviderKind, ListingRoute]] = {
    ProviderKind.OPENAI: ListingRoute(ListingMethod.LITELLM, litellm_provider="openai"),
    ProviderKind.ANTHROPIC: ListingRoute(ListingMethod.LITELLM, litellm_provider="anthropic"),
    ProviderKind.GOOGLE: ListingRoute(ListingMethod.LITELLM, litellm_provider="gemini"),
    ProviderKind.OLLAMA: ListingRoute(
        ListingMethod.LITELLM, litellm_provider="ollama", requires_api_key=False
    ),
    ProviderKind.OPENROUTER: ListingRoute(
        ListingMethod.OPENAI_SDK, default_base_url="https://openrouter.ai/api/v1"
    ),
    ProviderKind.OPENAI_COMPATIBLE: ListingRoute(ListingMethod.OPENAI_SDK, requires_api_key=False),
    ProviderKind.AZURE: ListingRoute(ListingMethod.MANUAL),
    ProviderKind.COHERE: ListingRoute(ListingMethod.MANUAL),
    ProviderKind.VOYAGE: ListingRoute(ListingMethod.MANUAL),
    ProviderKind.JINA: ListingRoute(ListingMethod.MANUAL),
}


class ModelLister(Protocol):
    async def list_models(self, access: ProviderAccess) -> list[str]: ...


def _key(access: ProviderAccess) -> str | None:
    return access.api_key.get_secret_value() if access.api_key is not None else None


def _required_field(field: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": "REQUIRED"}]})


def _litellm_models(provider: str, access: ProviderAccess) -> list[str]:
    # Imported here so the CLI sets LITELLM_LOCAL_MODEL_COST_MAP before LiteLLM's first import.
    from litellm.types.utils import LlmProviders
    from litellm.utils import ProviderConfigManager

    info = ProviderConfigManager.get_provider_model_info(model=None, provider=LlmProviders(provider))
    if info is None:
        raise AppError(ErrorCode.REGISTRY_SYNC_UNSUPPORTED)
    # get_models raises on HTTP errors, unlike get_valid_models, which hides a bad key behind an empty list.
    return list(info.get_models(api_key=_key(access), api_base=access.base_url or None))


class LiteLLMLister:
    async def list_models(self, access: ProviderAccess) -> list[str]:
        provider = LISTING_ROUTES[access.kind].litellm_provider
        return await asyncio.to_thread(_litellm_models, provider, access)


class OpenAICompatibleLister:
    async def list_models(self, access: ProviderAccess) -> list[str]:
        base_url = access.base_url or LISTING_ROUTES[access.kind].default_base_url
        if not base_url:
            raise _required_field("baseUrl")
        client = AsyncOpenAI(base_url=base_url, api_key=_key(access) or KEYLESS_PLACEHOLDER)
        try:
            return [model.id async for model in client.models.list()]
        finally:
            await client.close()


class ModelListing:
    def __init__(self, litellm: ModelLister, openai_sdk: ModelLister) -> None:
        self._listers = {ListingMethod.LITELLM: litellm, ListingMethod.OPENAI_SDK: openai_sdk}

    async def list_models(self, access: ProviderAccess) -> list[str]:
        route = LISTING_ROUTES[access.kind]
        if route.method is ListingMethod.MANUAL:
            raise AppError(ErrorCode.REGISTRY_SYNC_UNSUPPORTED)
        if route.requires_api_key and access.api_key is None:
            raise _required_field("apiKey")
        try:
            return await self._listers[route.method].list_models(access)
        except AppError:
            raise
        except Exception as exc:
            log.warning("model listing failed", kind=access.kind.value, error_type=type(exc).__name__)
            raise AppError(ErrorCode.REGISTRY_PROVIDER_REJECTED) from None
