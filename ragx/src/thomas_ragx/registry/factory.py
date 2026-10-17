from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.config import Config
from thomas_ragx.registry.catalog_listing import CatalogLister
from thomas_ragx.registry.listing import LiteLLMLister, ModelListing, OpenAICompatibleLister
from thomas_ragx.registry.model_check import LiteLLMModelChecker
from thomas_ragx.registry.model_info import describe_model
from thomas_ragx.registry.openrouter_listing import OpenRouterLister
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.registry.vault import Vault


def create_registry_service(config: Config, pool: Pool) -> LLMProviderService:
    return LLMProviderService(
        pool,
        Vault(config.vault_master_key),
        ModelListing(
            LiteLLMLister(),
            OpenAICompatibleLister(),
            OpenRouterLister(config.model_listing_timeout_seconds),
            CatalogLister(),
        ),
        LiteLLMModelChecker(config.model_probe_timeout_seconds),
        describe_model,
    )
