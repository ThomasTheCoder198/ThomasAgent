import base64

import pytest
from pydantic import SecretStr

from tests.integration.fakes import FakeChecker, FakeLister
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import DEV_EMBEDDING_DIMS, seed_dev_registry
from thomas_ragx.registry.listing import ModelListing
from thomas_ragx.registry.model import EMPTY_FACTS, Capability, ModelFacts, ProviderKind, Role
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.registry.vault import Vault

pytestmark = pytest.mark.integration
MASTER_KEY = SecretStr(base64.b64encode(b"k" * 32).decode())
API_KEY = SecretStr("sk-or-v1-integration-key")


def _facts(kind: ProviderKind, name: str) -> ModelFacts:
    if name.endswith("embed"):
        return ModelFacts(frozenset({Capability.EMBEDDING}), 1024, 8192, None, None)
    return EMPTY_FACTS


def _service(
    pool: Pool, lister: FakeLister | None = None, vault_key: SecretStr = MASTER_KEY
) -> LLMProviderService:
    lister = lister or FakeLister()
    return LLMProviderService(pool, Vault(vault_key), ModelListing(lister, lister), FakeChecker(), _facts)


async def test_provider_key_is_write_only_and_stored_encrypted(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    assert provider.has_api_key
    assert API_KEY.get_secret_value() not in repr(await service.list_providers())
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT api_key_ciphertext FROM providers")).fetchone()
    assert row is not None
    assert API_KEY.get_secret_value().encode() not in bytes(row["api_key_ciphertext"])


async def test_sync_upserts_models_and_keeps_operator_capability_edits(pool: Pool) -> None:
    lister = FakeLister(["acme-embed", "acme-chat"])
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    assert await service.sync_models(provider.id) == 2
    chat = next(m for m in await service.list_models(provider.id) if m.name == "acme-chat")
    await service.update_model(chat.id, frozenset({Capability.CHAT}), None)
    await service.sync_models(provider.id)
    models = {m.name: m for m in await service.list_models(provider.id)}
    assert models["acme-chat"].capabilities == frozenset({Capability.CHAT})
    assert models["acme-embed"].embedding_dims == 1024
    assert lister.calls[0].api_key is not None


async def test_manual_model_creation_never_calls_the_provider(pool: Pool) -> None:
    lister = FakeLister()
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.COHERE, "Cohere", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "rerank-v3.5", frozenset({Capability.RERANK}), None, None
    )
    assert model.capabilities == frozenset({Capability.RERANK})
    assert lister.calls == []


async def test_role_requires_capability_and_embedding_requires_dims(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(
        ProviderKind.OPENAI_COMPATIBLE, "Local", "http://127.0.0.1:9/v1", None
    )
    chat = await service.add_manual_model(provider.id, "chat", frozenset({Capability.CHAT}), None, None)
    no_dims = await service.add_manual_model(
        provider.id, "embed", frozenset({Capability.EMBEDDING}), None, None
    )
    with pytest.raises(AppError) as mismatch:
        await service.assign_role(Role.EMBEDDING, chat.id)
    assert mismatch.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    with pytest.raises(AppError) as missing_dims:
        await service.assign_role(Role.EMBEDDING, no_dims.id)
    assert missing_dims.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    await service.assign_role(Role.CHAT_FAST, chat.id)
    assert (await service.roles())[Role.CHAT_FAST] == chat


async def test_model_in_a_role_cannot_be_deleted(pool: Pool) -> None:
    service = _service(pool)
    model = await seed_dev_registry(service)
    with pytest.raises(AppError) as in_use:
        await service.delete_model(model.id)
    assert in_use.value.code is ErrorCode.LLM_MODEL_IN_USE
    with pytest.raises(AppError) as provider_in_use:
        await service.delete_provider(model.provider_id)
    assert provider_in_use.value.code is ErrorCode.LLM_MODEL_IN_USE


async def test_dev_seed_is_idempotent_and_offline(pool: Pool) -> None:
    lister = FakeLister()
    service = _service(pool, lister)
    first = await seed_dev_registry(service)
    second = await seed_dev_registry(service)
    assert first == second
    assert first.embedding_dims == DEV_EMBEDDING_DIMS
    assert lister.calls == []


async def test_rotated_master_key_cannot_use_stored_keys(pool: Pool) -> None:
    provider = await _service(pool).create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    rotated = _service(pool, FakeLister(["x"]), SecretStr(base64.b64encode(b"r" * 32).decode()))
    with pytest.raises(AppError) as failure:
        await rotated.sync_models(provider.id)
    assert failure.value.code is ErrorCode.INTERNAL_ERROR


async def test_duplicate_provider_name_is_reported(pool: Pool) -> None:
    service = _service(pool)
    await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    with pytest.raises(AppError) as taken:
        await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    assert taken.value.code is ErrorCode.LLM_NAME_TAKEN
