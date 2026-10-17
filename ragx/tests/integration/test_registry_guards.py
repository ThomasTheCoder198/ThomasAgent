"""Registry guards from the Codex verification: key/URL binding, dims immutability, role race."""

import asyncio
import base64
import uuid
from uuid import UUID

import pytest
from pydantic import SecretStr

from tests.integration.fakes import MEASURED_DIMS, FakeChecker, FakeLister
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry import repository
from thomas_ragx.registry.listing import ListedModel, ModelListing
from thomas_ragx.registry.model import EMPTY_FACTS, Capability, ModelFacts, ProviderKind, Role
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.registry.vault import Vault

pytestmark = pytest.mark.integration
MASTER_KEY = SecretStr(base64.b64encode(b"k" * 32).decode())
OLD_KEY = SecretStr("sk-or-v1-old-key-must-not-travel")
NEW_KEY = SecretStr("sk-or-v1-new-key")
PROXY_URL = "https://proxy.example/api/v1"
EMBEDDING = frozenset({Capability.EMBEDDING})


def _service(
    pool: Pool, lister: FakeLister | None = None, checker: FakeChecker | None = None
) -> LLMProviderService:
    lister = lister or FakeLister()
    return LLMProviderService(
        pool,
        Vault(MASTER_KEY),
        ModelListing(lister, lister, lister, lister),
        checker or FakeChecker(),
        lambda kind, name: EMPTY_FACTS,
    )


async def _invalid_base_url(call: object) -> None:
    with pytest.raises(AppError) as failure:
        await call  # type: ignore[misc]
    assert failure.value.code is ErrorCode.VALIDATION_FAILED
    assert failure.value.details == {"fields": [{"field": "baseUrl", "code": "INVALID"}]}


async def _kb_using(pool: Pool, model_id: UUID, dims: int) -> None:
    async with pool.connection() as conn:
        await conn.execute(b"INSERT INTO orgs (id, name, external_parsing) VALUES ('o', 'O', false)")
        await conn.execute(
            b"INSERT INTO knowledge_bases (id, org_id, name, is_default, embedding_model_id, embedding_dims, "
            b"collection_name) VALUES (%s, 'o', 'kb', true, %s, %s, 'c')",
            (uuid.uuid4(), model_id, dims),
        )


# ---- C1: a key never follows an edit to another address ----


async def test_an_old_key_never_reaches_a_changed_address(pool: Pool) -> None:
    lister = FakeLister([ListedModel("acme/embed", ModelFacts(EMBEDDING, None, 1, None, None))])
    checker = FakeChecker()
    service = _service(pool, lister, checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", OLD_KEY)
    edited = await service.update_provider(provider.id, "OpenRouter", PROXY_URL, None, clear_api_key=False)
    assert edited.has_api_key is False
    await service.sync_models(provider.id)
    (model,) = await service.list_models(provider.id)
    await service.test_model(model.id)
    assert [call.api_key for call in lister.calls] == [None]
    assert [access.api_key for access, _ in checker.measured] == [None]
    assert {call.base_url for call in lister.calls} == {PROXY_URL}


async def test_a_new_key_in_the_same_edit_is_the_only_key_that_follows(pool: Pool) -> None:
    lister = FakeLister(["acme/chat"])
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", OLD_KEY)
    edited = await service.update_provider(provider.id, "OpenRouter", PROXY_URL, NEW_KEY, clear_api_key=False)
    assert edited.has_api_key
    await service.sync_models(provider.id)
    key = lister.calls[0].api_key
    assert key is not None
    assert key.get_secret_value() == NEW_KEY.get_secret_value()


async def test_an_unchanged_address_keeps_the_key_on_a_rename(pool: Pool) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", PROXY_URL, OLD_KEY)
    renamed = await service.update_provider(
        provider.id, "Renamed", PROXY_URL + "/", None, clear_api_key=False
    )
    assert renamed.has_api_key


@pytest.mark.parametrize(
    "kind",
    [
        ProviderKind.OPENROUTER,
        ProviderKind.OPENAI,
        ProviderKind.ANTHROPIC,
        ProviderKind.GOOGLE,
        ProviderKind.COHERE,
        ProviderKind.VOYAGE,
        ProviderKind.JINA,
        ProviderKind.AZURE,
    ],
)
async def test_a_built_in_kind_accepts_only_https(pool: Pool, kind: ProviderKind) -> None:
    service = _service(pool)
    await _invalid_base_url(service.create_provider(kind, "Plain http", "http://proxy.example/v1", OLD_KEY))
    provider = await service.create_provider(kind, "Https", PROXY_URL, OLD_KEY)
    await _invalid_base_url(
        service.update_provider(provider.id, "Https", "http://proxy.example/v1", None, clear_api_key=False)
    )


@pytest.mark.parametrize("kind", [ProviderKind.OLLAMA, ProviderKind.OPENAI_COMPATIBLE])
async def test_a_self_hosted_kind_may_use_http_and_private_hosts(pool: Pool, kind: ProviderKind) -> None:
    service = _service(pool)
    for url in ("http://host.docker.internal:11434", "http://127.0.0.1:9/v1", "http://10.0.0.5/v1"):
        provider = await service.create_provider(kind, f"Local {url}", url, None)
        assert provider.base_url == url


@pytest.mark.parametrize(
    "url",
    [
        "https://user:pw@proxy.example/v1",
        "https://user@proxy.example/v1",
        "https://",
        "https:///v1",
        "https://proxy.example:notaport/v1",
        "https://proxy .example/v1",
        "ftp://proxy.example",
        "proxy.example/v1",
    ],
)
async def test_a_malformed_or_credentialed_address_is_invalid(pool: Pool, url: str) -> None:
    await _invalid_base_url(_service(pool).create_provider(ProviderKind.AZURE, "Azure", url, OLD_KEY))


async def test_self_hosted_userinfo_is_invalid_too(pool: Pool) -> None:
    await _invalid_base_url(
        _service(pool).create_provider(ProviderKind.OLLAMA, "Ollama", "http://user:pw@10.0.0.5:11434", None)
    )


# ---- C2: dims of a model a KB uses never move ----


async def test_a_kb_backed_model_keeps_its_dims_through_edit_sync_and_probe(pool: Pool) -> None:
    listed = ListedModel("acme/embed", ModelFacts(EMBEDDING, 999, 8192, None, None))
    checker = FakeChecker(dims=999)
    service = _service(pool, FakeLister([listed]), checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", OLD_KEY)
    model = await service.add_manual_model(provider.id, "acme/embed", EMBEDDING, 1536, None)
    await _kb_using(pool, model.id, dims=1536)
    edited = await service.update_model(model.id, frozenset({Capability.EMBEDDING, Capability.CHAT}))
    assert edited.embedding_dims == 1536
    await service.sync_models(provider.id)
    async with pool.connection() as conn:
        assert await repository.set_measured_dims(conn, model.id, 999) is False
    await service.test_model(model.id)
    await service.assign_role(Role.EMBEDDING, model.id)
    (after,) = await service.list_models(provider.id)
    assert after.embedding_dims == 1536
    assert checker.measured == []


async def test_a_resync_cannot_fill_dims_of_a_model_a_kb_already_uses(pool: Pool) -> None:
    listed = ListedModel("acme/embed", ModelFacts(EMBEDDING, 999, 8192, None, None))
    service = _service(pool, FakeLister([listed]))
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", OLD_KEY)
    model = await service.add_manual_model(provider.id, "acme/embed", EMBEDDING, None, None)
    await _kb_using(pool, model.id, dims=1536)
    async with pool.connection() as conn:
        await conn.execute(b"UPDATE models SET source = 'sync'")
    await service.sync_models(provider.id)
    (after,) = await service.list_models(provider.id)
    assert after.embedding_dims is None


# ---- I3: the role is judged after the probe, under a row lock ----


async def _paused_assign(
    pool: Pool, dims: int = MEASURED_DIMS
) -> tuple[LLMProviderService, UUID, UUID, FakeChecker, asyncio.Event]:
    gate = asyncio.Event()
    checker = FakeChecker(dims=dims, gate=gate)
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", OLD_KEY)
    model = await service.add_manual_model(provider.id, "acme/embed", EMBEDDING, None, None)
    return service, provider.id, model.id, checker, gate


async def test_a_capability_edit_during_the_probe_refuses_the_role(pool: Pool) -> None:
    service, provider_id, model_id, checker, gate = await _paused_assign(pool)
    assigning = asyncio.create_task(service.assign_role(Role.EMBEDDING, model_id))
    await checker.probing.wait()
    await service.update_model(model_id, frozenset({Capability.CHAT}))
    gate.set()
    with pytest.raises(AppError) as failure:
        await assigning
    assert failure.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    assert (await service.roles())[Role.EMBEDDING] is None
    (model,) = await service.list_models(provider_id)
    assert model.embedding_dims is None


async def test_a_competing_probe_with_another_size_refuses_the_role(pool: Pool) -> None:
    service, _, model_id, checker, gate = await _paused_assign(pool, dims=768)
    assigning = asyncio.create_task(service.assign_role(Role.EMBEDDING, model_id))
    await checker.probing.wait()
    async with pool.connection() as conn:
        assert await repository.set_measured_dims(conn, model_id, 512)
    gate.set()
    with pytest.raises(AppError) as failure:
        await assigning
    assert failure.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH
    assert (await service.roles())[Role.EMBEDDING] is None


async def test_a_competing_probe_with_the_same_size_lets_the_role_through(pool: Pool) -> None:
    service, _, model_id, checker, gate = await _paused_assign(pool, dims=512)
    assigning = asyncio.create_task(service.assign_role(Role.EMBEDDING, model_id))
    await checker.probing.wait()
    async with pool.connection() as conn:
        assert await repository.set_measured_dims(conn, model_id, 512)
    gate.set()
    await assigning
    assigned = (await service.roles())[Role.EMBEDDING]
    assert assigned is not None
    assert assigned.embedding_dims == 512


async def test_an_address_change_during_the_probe_refuses_the_role(pool: Pool) -> None:
    service, provider_id, model_id, checker, gate = await _paused_assign(pool)
    assigning = asyncio.create_task(service.assign_role(Role.EMBEDDING, model_id))
    await checker.probing.wait()
    await service.update_provider(provider_id, "OpenRouter", PROXY_URL, NEW_KEY, clear_api_key=False)
    gate.set()
    with pytest.raises(AppError) as failure:
        await assigning
    assert failure.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    assert (await service.roles())[Role.EMBEDDING] is None
    (model,) = await service.list_models(provider_id)
    assert model.embedding_dims is None


# ---- I4: a KB cannot be built from a model with no dims ----


async def test_a_dimensionless_role_model_is_a_catalog_error_not_a_crash(pool: Pool) -> None:
    from tests.integration.api_client import api_client, bearer

    async with api_client(pool) as (client, app):
        service: LLMProviderService = app.state.registry_service
        provider = await service.create_provider(
            ProviderKind.OPENAI_COMPATIBLE, "Local", "http://127.0.0.1:9/v1", None
        )
        model = await service.add_manual_model(provider.id, "embed", EMBEDDING, 1536, None)
        await service.assign_role(Role.EMBEDDING, model.id)
        provision = {"name": "Alpha", "externalParsing": False, "settingsVersion": 1}
        first = await client.put("/v1/orgs/org-a", json=provision, headers=bearer("org-a", "org:provision"))
        assert first.status_code == 200
        async with pool.connection() as conn:
            await conn.execute(b"UPDATE models SET embedding_dims = NULL")
        created = await client.post(
            "/v1/knowledge-bases", json={"name": "Second"}, headers=bearer("org-a", "kb:write")
        )
        provisioned = await client.put(
            "/v1/orgs/org-b", json=provision, headers=bearer("org-b", "org:provision")
        )
    assert created.status_code == 422
    assert created.json()["error"]["code"] == ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH.value
    assert provisioned.status_code == 422
    assert provisioned.json()["error"]["code"] == ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH.value


# ---- Lock order: provider, then model. Edits and probes serialize on the provider row ----

OTHER_URL = "https://other.example/api/v1"


async def _wait_for_a_lock_wait(pool: Pool) -> None:
    """Observes the schedule instead of sleeping: a backend is blocked on a row lock."""
    for _ in range(200):
        async with pool.connection() as conn:
            row = await (
                await conn.execute(
                    b"SELECT count(*) AS waiting FROM pg_stat_activity "
                    b"WHERE wait_event_type = 'Lock' AND datname = current_database()"
                )
            ).fetchone()
        if row is not None and row["waiting"]:
            return
        await asyncio.sleep(0.05)
    raise AssertionError("no backend ever waited on a lock")


async def test_concurrent_address_edits_never_leave_a_key_that_belongs_to_another_address(
    pool: Pool, monkeypatch: pytest.MonkeyPatch
) -> None:
    service = _service(pool)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", PROXY_URL, OLD_KEY)
    paused, release = asyncio.Event(), asyncio.Event()
    real_update = repository.update_provider
    calls = 0

    async def pausing_update(*args: object, **kwargs: object) -> object:
        nonlocal calls
        calls += 1
        if calls == 1:  # edit B holds the provider lock here, having read the old address
            paused.set()
            await release.wait()
        return await real_update(*args, **kwargs)  # type: ignore[arg-type]

    monkeypatch.setattr(repository, "update_provider", pausing_update)
    edit_b = asyncio.create_task(
        service.update_provider(provider.id, "OpenRouter", OTHER_URL, NEW_KEY, clear_api_key=False)
    )
    await paused.wait()
    # Edit A comes back to the address it read and submits no key.
    edit_a = asyncio.create_task(
        service.update_provider(provider.id, "OpenRouter", PROXY_URL, None, clear_api_key=False)
    )
    await _wait_for_a_lock_wait(pool)
    release.set()
    await asyncio.gather(edit_b, edit_a)
    (final,) = await service.list_providers()
    assert final.base_url == PROXY_URL
    assert final.has_api_key is False


async def test_an_address_edit_waits_for_the_role_commit_and_the_role_keeps_the_probed_address(
    pool: Pool, monkeypatch: pytest.MonkeyPatch
) -> None:
    checker = FakeChecker(dims=512)
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", PROXY_URL, OLD_KEY)
    model = await service.add_manual_model(provider.id, "acme/embed", EMBEDDING, None, None)
    inside, release = asyncio.Event(), asyncio.Event()
    real_assign = repository.assign_role

    async def holding_assign(*args: object, **kwargs: object) -> None:
        inside.set()  # the provider and model locks are held from here to the commit
        await release.wait()
        await real_assign(*args, **kwargs)  # type: ignore[arg-type]

    monkeypatch.setattr(repository, "assign_role", holding_assign)
    assigning = asyncio.create_task(service.assign_role(Role.EMBEDDING, model.id))
    await inside.wait()
    editing = asyncio.create_task(
        service.update_provider(provider.id, "OpenRouter", OTHER_URL, NEW_KEY, clear_api_key=False)
    )
    await _wait_for_a_lock_wait(pool)
    assert not editing.done()
    release.set()
    await asyncio.gather(assigning, editing)
    assigned = (await service.roles())[Role.EMBEDDING]
    assert assigned is not None
    assert assigned.embedding_dims == 512
    assert [access.base_url for access, _ in checker.measured] == [PROXY_URL]
    (after,) = await service.list_providers()
    assert after.base_url == OTHER_URL


async def test_a_sync_listed_from_an_old_address_is_not_stored_after_the_address_changed(
    pool: Pool,
) -> None:
    lister = FakeLister(["acme/chat"])
    service = _service(pool, lister)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", PROXY_URL, OLD_KEY)

    class EditingLister(FakeLister):
        async def list_models(self, access):  # type: ignore[no-untyped-def]
            await service.update_provider(provider.id, "OpenRouter", OTHER_URL, NEW_KEY, clear_api_key=False)
            return await super().list_models(access)

    editing = EditingLister(["acme/chat"])
    service = _service(pool, editing)
    with pytest.raises(AppError) as failure:
        await service.sync_models(provider.id)
    assert failure.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    assert await service.list_models(provider.id) == []


async def test_a_passing_test_is_refused_when_the_address_changed_during_the_check(pool: Pool) -> None:
    gate = asyncio.Event()

    class PausedCheck(FakeChecker):
        async def check(self, access, model) -> None:  # type: ignore[no-untyped-def]
            await super().check(access, model)
            self.probing.set()
            await gate.wait()

    checker = PausedCheck()
    service = _service(pool, checker=checker)
    provider = await service.create_provider(ProviderKind.OPENROUTER, "OpenRouter", PROXY_URL, OLD_KEY)
    model = await service.add_manual_model(provider.id, "acme/embed", EMBEDDING, 1536, None)
    testing = asyncio.create_task(service.test_model(model.id))
    await checker.probing.wait()
    await service.update_provider(provider.id, "OpenRouter", OTHER_URL, NEW_KEY, clear_api_key=False)
    gate.set()
    with pytest.raises(AppError) as failure:
        await testing
    assert failure.value.code is ErrorCode.LLM_PROVIDER_REJECTED
