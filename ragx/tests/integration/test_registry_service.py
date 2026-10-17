import base64
import uuid
from collections.abc import Awaitable, Callable
from decimal import Decimal
from uuid import UUID

import pytest
from pydantic import SecretStr

from tests.integration.fakes import MEASURED_DIMS, FakeChecker, FakeLister
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry import repository
from thomas_ragx.registry.catalog_listing import CatalogLister
from thomas_ragx.registry.dev_seed import DEV_EMBEDDING_DIMS, seed_dev_registry
from thomas_ragx.registry.listing import ListedModel, ModelListing
from thomas_ragx.registry.model import (
    EMPTY_FACTS,
    KIND_RULES,
    Capability,
    ModelFacts,
    ProviderKind,
    Role,
)
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.registry.vault import Vault

pytestmark = pytest.mark.integration
MASTER_KEY = SecretStr(base64.b64encode(b"k" * 32).decode())
API_KEY = SecretStr("sk-or-v1-integration-key")
LOCAL_URL = "http://127.0.0.1:9/v1"
KEYED_KINDS = [kind for kind, rules in KIND_RULES.items() if rules.api_key_required]


def _facts(kind: ProviderKind, name: str) -> ModelFacts:
    if name.endswith("embed"):
        return ModelFacts(frozenset({Capability.EMBEDDING}), 1024, 8192, None, None)
    return EMPTY_FACTS


def _no_describe(kind: ProviderKind, name: str) -> ModelFacts:
    raise AssertionError("a listing that carries facts must not trigger a LiteLLM lookup")


def _service(
    pool: Pool,
    lister: FakeLister | None = None,
    vault_key: SecretStr = MASTER_KEY,
    checker: FakeChecker | None = None,
    catalog: CatalogLister | None = None,
    describe: Callable[[ProviderKind, str], ModelFacts] = _facts,
) -> LLMProviderService:
    lister = lister or FakeLister()
    listing = ModelListing(lister, lister, lister, catalog or lister)
    return LLMProviderService(pool, Vault(vault_key), listing, checker or FakeChecker(), describe)


async def _field_errors(call: Awaitable[object]) -> list[dict[str, str]]:
    with pytest.raises(AppError) as failure:
        await call
    assert failure.value.code is ErrorCode.VALIDATION_FAILED
    assert failure.value.details is not None
    return failure.value.details["fields"]


async def _keyless_openrouter(service: LLMProviderService, name: str = "OpenRouter") -> UUID:
    """A row from before the key became mandatory: created with a key, then cleared."""
    provider = await service.create_provider(ProviderKind.OPENROUTER, name, "", API_KEY)
    await service.update_provider(provider.id, name, "", None, clear_api_key=True)
    return provider.id


async def _kb_using(pool: Pool, model_id: UUID, dims: int) -> None:
    async with pool.connection() as conn:
        await conn.execute(b"INSERT INTO orgs (id, name, external_parsing) VALUES ('o', 'O', false)")
        await conn.execute(
            b"INSERT INTO knowledge_bases (id, org_id, name, is_default, embedding_model_id, embedding_dims, "
            b"collection_name) VALUES (%s, 'o', 'kb', true, %s, %s, 'c')",
            (uuid.uuid4(), model_id, dims),
        )


async def test_provider_key_is_write_only_and_stored_encrypted(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    assert provider.has_api_key
    assert API_KEY.get_secret_value() not in repr(await service.list_providers())
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT api_key_ciphertext FROM providers")).fetchone()
    assert row is not None
    assert API_KEY.get_secret_value().encode() not in bytes(row["api_key_ciphertext"])


@pytest.mark.parametrize("kind", KEYED_KINDS)
async def test_a_built_in_kind_needs_a_key_at_creation(pool: Pool, kind: ProviderKind) -> None:
    # Azure's base URL is also required, so give it one and isolate the key rule.
    errors = await _field_errors(
        _service(pool).create_provider(kind, "No key", "https://azure.example", None)
    )
    assert {"field": "apiKey", "code": "REQUIRED"} in errors


@pytest.mark.parametrize("kind", [ProviderKind.OLLAMA, ProviderKind.OPENAI_COMPATIBLE])
async def test_a_local_server_kind_needs_no_key(pool: Pool, kind: ProviderKind) -> None:
    provider = await _service(pool).create_provider(kind, "Local", LOCAL_URL, None)
    assert not provider.has_api_key


@pytest.mark.parametrize("kind", [ProviderKind.OPENAI_COMPATIBLE, ProviderKind.AZURE])
async def test_base_url_is_required_for_openai_compatible_and_azure(pool: Pool, kind: ProviderKind) -> None:
    errors = await _field_errors(_service(pool).create_provider(kind, "No URL", "", API_KEY))
    assert errors == [{"field": "baseUrl", "code": "REQUIRED"}]


async def test_every_missing_field_is_reported_at_once(pool: Pool) -> None:
    errors = await _field_errors(_service(pool).create_provider(ProviderKind.AZURE, "Both", "", None))
    assert errors == [{"field": "apiKey", "code": "REQUIRED"}, {"field": "baseUrl", "code": "REQUIRED"}]


async def test_a_built_in_kind_and_ollama_need_no_base_url(pool: Pool) -> None:
    service = _service(pool)
    await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    await service.create_provider(ProviderKind.OLLAMA, "Ollama", "", None)


async def test_a_keyless_old_row_can_be_edited_and_update_keeps_an_existing_key(pool: Pool) -> None:
    service = _service(pool)
    legacy_id = await _keyless_openrouter(service)
    renamed = await service.update_provider(legacy_id, "OpenRouter 2", "", None, clear_api_key=False)
    assert (renamed.name, renamed.has_api_key) == ("OpenRouter 2", False)
    fixed = await service.update_provider(legacy_id, "OpenRouter 2", "", API_KEY, clear_api_key=False)
    assert fixed.has_api_key
    kept = await service.update_provider(legacy_id, "OpenRouter 3", "", None, clear_api_key=False)
    assert kept.has_api_key


async def test_update_keeps_the_base_url_rule(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENAI_COMPATIBLE, "Local", LOCAL_URL, None)
    errors = await _field_errors(service.update_provider(provider.id, "Local", "", None, clear_api_key=False))
    assert errors == [{"field": "baseUrl", "code": "REQUIRED"}]


async def test_sync_upserts_models_and_keeps_operator_capability_edits(pool: Pool) -> None:
    lister = FakeLister(["acme-embed", "acme-chat"])
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    assert await service.sync_models(provider.id) == 2
    chat = next(m for m in await service.list_models(provider.id) if m.name == "acme-chat")
    await service.update_model(chat.id, frozenset({Capability.CHAT}))
    await service.sync_models(provider.id)
    models = {m.name: m for m in await service.list_models(provider.id)}
    assert models["acme-chat"].capabilities == frozenset({Capability.CHAT})
    assert models["acme-embed"].embedding_dims == 1024
    assert lister.calls[0].api_key is not None


async def test_a_listing_with_facts_stores_them_and_skips_the_litellm_lookup(pool: Pool) -> None:
    listed = ListedModel(
        "acme/thinker",
        ModelFacts(
            frozenset({Capability.CHAT, Capability.TOOLS, Capability.REASONING}),
            None,
            200000,
            Decimal("0.000003"),
            Decimal("0.000015"),
        ),
    )
    service = _service(pool, FakeLister([listed]), describe=_no_describe)
    provider_id = await _keyless_openrouter(service)
    assert await service.sync_models(provider_id) == 1
    (model,) = await service.list_models(provider_id)
    assert model.capabilities == {Capability.CHAT, Capability.TOOLS, Capability.REASONING}
    assert model.context_window == 200000
    assert model.input_cost_per_token == Decimal("0.000003")


async def test_keyless_openrouter_lists_without_a_key(pool: Pool) -> None:
    lister = FakeLister(
        [ListedModel("acme/chat", ModelFacts(frozenset({Capability.CHAT}), None, 1, None, None))]
    )
    service = _service(pool, lister)
    await service.sync_models(await _keyless_openrouter(service))
    assert lister.calls[0].api_key is None


async def test_a_catalog_sync_fills_rerank_and_embedding_models_without_a_network_call(pool: Pool) -> None:
    lister = FakeLister()
    service = _service(pool, lister, catalog=CatalogLister())
    provider = await service.create_provider(ProviderKind.COHERE, "Cohere", "", API_KEY)
    assert await service.sync_models(provider.id) > 0
    models = {m.name: m for m in await service.list_models(provider.id)}
    assert models["rerank-v3.5"].capabilities == {Capability.RERANK}
    assert models["embed-v4.0"].embedding_dims == 1536
    assert lister.calls == []
    await service.assign_role(Role.RERANK, models["rerank-v3.5"].id)
    assert (await service.roles())[Role.RERANK] == models["rerank-v3.5"]


async def test_a_resync_never_wipes_measured_dims(pool: Pool) -> None:
    listed = ListedModel(
        "acme/embed-x", ModelFacts(frozenset({Capability.EMBEDDING}), None, 8192, None, None)
    )
    service = _service(pool, FakeLister([listed]))
    provider_id = await _keyless_openrouter(service)
    await service.sync_models(provider_id)
    (model,) = await service.list_models(provider_id)
    await service.assign_role(Role.EMBEDDING, model.id)
    await service.sync_models(provider_id)
    (after,) = await service.list_models(provider_id)
    assert after.embedding_dims == MEASURED_DIMS
    assert after.capabilities_edited is False


async def test_a_capability_edit_keeps_the_dims(pool: Pool) -> None:
    service = _service(pool)
    model = await seed_dev_registry(service)
    edited = await service.update_model(model.id, frozenset({Capability.EMBEDDING, Capability.CHAT}))
    assert edited.embedding_dims == DEV_EMBEDDING_DIMS


async def test_manual_model_creation_never_calls_the_provider(pool: Pool) -> None:
    lister = FakeLister()
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.COHERE, "Cohere", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "rerank-v3.5", frozenset({Capability.RERANK}), None, None
    )
    assert model.capabilities == frozenset({Capability.RERANK})
    assert lister.calls == []


async def test_role_requires_a_capable_model(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENAI_COMPATIBLE, "Local", LOCAL_URL, None)
    chat = await service.add_manual_model(provider.id, "chat", frozenset({Capability.CHAT}), None, None)
    with pytest.raises(AppError) as mismatch:
        await service.assign_role(Role.EMBEDDING, chat.id)
    assert mismatch.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    await service.assign_role(Role.CHAT_FAST, chat.id)
    assert (await service.roles())[Role.CHAT_FAST] == chat


async def test_assigning_the_embedding_role_measures_missing_dims(pool: Pool) -> None:
    checker = FakeChecker(dims=3072)
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "embed-me", frozenset({Capability.EMBEDDING}), None, None
    )
    await service.assign_role(Role.EMBEDDING, model.id)
    assigned = (await service.roles())[Role.EMBEDDING]
    assert assigned is not None
    assert (assigned.id, assigned.embedding_dims) == (model.id, 3072)
    (probe_access, _) = checker.measured[0]
    assert probe_access.api_key is not None


async def test_a_model_with_known_dims_is_not_probed_for_the_role(pool: Pool) -> None:
    checker = FakeChecker()
    service = _service(pool, checker=checker)
    await seed_dev_registry(service)
    assert checker.measured == []


async def test_measuring_is_not_an_operator_edit(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "embed-me", frozenset({Capability.EMBEDDING}), None, None
    )
    async with pool.connection() as conn:
        await conn.execute(b"UPDATE models SET capabilities_edited = false")
        assert await repository.set_measured_dims(conn, model.id, 512)
        stored = await repository.get_model(conn, model.id)
    assert stored is not None
    assert (stored.embedding_dims, stored.capabilities_edited) == (512, False)


async def test_a_probe_failure_rejects_the_role_and_assigns_nothing(pool: Pool) -> None:
    checker = FakeChecker(error=AppError(ErrorCode.LLM_PROVIDER_REJECTED))
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "embed-me", frozenset({Capability.EMBEDDING}), None, None
    )
    with pytest.raises(AppError) as rejected:
        await service.assign_role(Role.EMBEDDING, model.id)
    assert rejected.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    assert (await service.roles())[Role.EMBEDDING] is None
    (unchanged,) = await service.list_models(provider.id)
    assert unchanged.embedding_dims is None


async def test_dims_of_a_model_a_kb_uses_are_never_changed(pool: Pool) -> None:
    service = _service(pool, checker=FakeChecker(dims=999))
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    model = await service.add_manual_model(
        provider.id, "embed-me", frozenset({Capability.EMBEDDING}), None, None
    )
    await _kb_using(pool, model.id, dims=1536)
    async with pool.connection() as conn:
        assert await repository.set_measured_dims(conn, model.id, 999) is False
    with pytest.raises(AppError) as mismatch:
        await service.assign_role(Role.EMBEDDING, model.id)
    assert mismatch.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    (unchanged,) = await service.list_models(provider.id)
    assert unchanged.embedding_dims is None


async def test_the_test_button_records_missing_dims_and_never_overwrites_known_ones(pool: Pool) -> None:
    checker = FakeChecker(dims=384)
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
    unknown = await service.add_manual_model(
        provider.id, "embed-a", frozenset({Capability.EMBEDDING}), None, None
    )
    known = await service.add_manual_model(
        provider.id, "embed-b", frozenset({Capability.EMBEDDING}), 1024, None
    )
    await service.test_model(unknown.id)
    await service.test_model(known.id)
    dims = {m.name: m.embedding_dims for m in await service.list_models(provider.id)}
    assert dims == {"embed-a": 384, "embed-b": 1024}
    assert len(checker.measured) == 1
    assert len(checker.calls) == 1


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
