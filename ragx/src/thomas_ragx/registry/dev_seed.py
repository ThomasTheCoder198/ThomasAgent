from typing import Final

from thomas_ragx.registry.model import Capability, ModelRecord, ProviderKind, Role
from thomas_ragx.registry.service import LLMProviderService

DEV_PROVIDER_NAME: Final = "Dev seed (offline)"
# A closed local port keeps the offline seed from reaching a provider.
DEV_PROVIDER_BASE_URL: Final = "http://127.0.0.1:9/v1"
DEV_EMBEDDING_MODEL: Final = "dev-embedding"
DEV_EMBEDDING_DIMS: Final = 1536


async def seed_dev_registry(service: LLMProviderService) -> ModelRecord:
    """Idempotent: provider, one embedding model and the embedding role, without any network call."""
    provider = next((p for p in await service.list_providers() if p.name == DEV_PROVIDER_NAME), None)
    if provider is None:
        provider = await service.create_provider(
            ProviderKind.OPENAI_COMPATIBLE, DEV_PROVIDER_NAME, DEV_PROVIDER_BASE_URL, None
        )
    model = next((m for m in await service.list_models(provider.id) if m.name == DEV_EMBEDDING_MODEL), None)
    if model is None:
        model = await service.add_manual_model(
            provider.id, DEV_EMBEDDING_MODEL, frozenset({Capability.EMBEDDING}), DEV_EMBEDDING_DIMS, None
        )
    if (await service.roles())[Role.EMBEDDING] is None:
        await service.assign_role(Role.EMBEDDING, model.id)
    return model
