from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.config import Config
from thomas_ragx.registry.listing import LiteLLMLister, ModelListing, OpenAICompatibleLister
from thomas_ragx.registry.model_check import LiteLLMModelChecker
from thomas_ragx.registry.model_info import describe_model
from thomas_ragx.registry.service import RegistryService
from thomas_ragx.registry.vault import Vault


def create_registry_service(config: Config, pool: Pool) -> RegistryService:
    return RegistryService(
        pool,
        Vault(config.vault_master_key),
        ModelListing(LiteLLMLister(), OpenAICompatibleLister()),
        LiteLLMModelChecker(),
        describe_model,
    )
