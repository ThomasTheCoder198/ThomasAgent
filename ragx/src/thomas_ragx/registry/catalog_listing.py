from collections.abc import Mapping
from typing import Any, Final

from thomas_ragx.registry.listing import LISTING_ROUTES, ListedModel
from thomas_ragx.registry.model import ProviderAccess
from thomas_ragx.registry.model_info import MODE_CAPABILITIES, facts_from_info

PROVIDER_FIELD: Final = "litellm_provider"
MODE_FIELD: Final = "mode"


def _catalog_entries(provider: str) -> list[ListedModel]:
    # Imported here so the CLI sets LITELLM_LOCAL_MODEL_COST_MAP before LiteLLM's first import.
    import litellm

    prefix = f"{provider}/"
    listed: dict[str, ListedModel] = {}
    cost_map: Mapping[str, Any] = litellm.model_cost
    for key, info in cost_map.items():
        # Modes outside embedding/rerank/chat (Cohere `command*` completion, OCR) are not registry models.
        if (
            not isinstance(info, Mapping)
            or info.get(PROVIDER_FIELD) != provider
            or info.get(MODE_FIELD) not in MODE_CAPABILITIES
        ):
            continue
        # litellm_model_name adds the provider prefix again, so the stored name must not carry it.
        name = key.removeprefix(prefix)
        listed[name] = ListedModel(name, facts_from_info(info))
    return list(listed.values())


class CatalogLister:
    """Providers without a useful list endpoint: LiteLLM's bundled map, so Sync makes no network call."""

    async def list_models(self, access: ProviderAccess) -> list[ListedModel]:
        return _catalog_entries(LISTING_ROUTES[access.kind].litellm_provider)
