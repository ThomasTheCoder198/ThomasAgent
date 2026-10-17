import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any
from urllib.parse import parse_qs, urlsplit
from uuid import UUID

import httpx
import pytest
from pydantic import SecretStr

from tests.conftest import TEST_VAULT_MASTER_KEY
from tests.integration.api_client import api_client
from tests.integration.fakes import MEASURED_DIMS, FakeChecker, FakeLister
from tests.integration.test_admin_ui import EMAIL, PASSWORD, REQUEST_ID, _csrf, _login
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.listing import ListedModel, ModelListing
from thomas_ragx.registry.model import EMPTY_FACTS, Capability, ModelFacts, ProviderKind, Role
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.registry.vault import Vault

pytestmark = pytest.mark.integration
API_KEY = SecretStr("sk-or-v1-ui-test-key")
MODEL_NAME = re.compile(r'<h2 class="tabular model-name">([^<]+)</h2>')


@asynccontextmanager
async def _admin(
    pool: Pool,
    redis_url: str,
    lister: FakeLister | None = None,
    checker: FakeChecker | None = None,
    **overrides: Any,
) -> AsyncIterator[tuple[httpx.AsyncClient, LLMProviderService, str]]:
    """A logged-in admin whose registry lists and probes through fakes, so no test reaches a provider."""
    async with api_client(pool, redis_url=redis_url, **overrides) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        await _login(client)
        lister = lister or FakeLister()
        registry = LLMProviderService(
            pool,
            Vault(SecretStr(TEST_VAULT_MASTER_KEY)),
            ModelListing(lister, lister, lister, lister),
            checker or FakeChecker(),
            lambda kind, name: EMPTY_FACTS,
        )
        app.state.registry_service = registry
        client.headers["accept-language"] = "en"
        yield client, registry, await _csrf(client, "/ui/providers")


async def _keyless(registry: LLMProviderService, kind: ProviderKind, name: str) -> UUID:
    """A row from before the key became mandatory: created with a key, then cleared."""
    provider = await registry.create_provider(kind, name, "", API_KEY)
    await registry.update_provider(provider.id, name, "", None, clear_api_key=True)
    return provider.id


def _facts(*capabilities: Capability, context: int | None = None) -> ModelFacts:
    return ModelFacts(frozenset(capabilities), None, context, None, None)


async def test_a_keyless_built_in_provider_is_refused_on_the_form_with_what_was_typed(
    pool: Pool, redis_url: str
) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        response = await client.post(
            "/ui/providers",
            data={"csrf_token": token, "kind": "openrouter", "name": "My OR"},
            follow_redirects=False,
        )
        providers = await registry.list_providers()
    assert response.status_code == 400
    assert "needs an API key" in response.text
    assert 'value="My OR"' in response.text
    assert re.search(r'value="openrouter"[^>]*selected', response.text)
    assert REQUEST_ID.search(response.text)
    assert providers == []


async def test_a_refused_provider_form_never_sends_the_key_back(pool: Pool, redis_url: str) -> None:
    secret = "sk-azure-never-echo-me"  # noqa: S105 - must never appear in HTML
    async with _admin(pool, redis_url) as (client, _, token):
        response = await client.post(
            "/ui/providers",
            data={"csrf_token": token, "kind": "azure", "name": "Azure", "api_key": secret},
            follow_redirects=False,
        )
    assert response.status_code == 400
    assert "needs a Base URL" in response.text
    assert secret not in response.text


async def test_the_provider_form_hides_the_base_url_through_data_attributes_only(
    pool: Pool, redis_url: str
) -> None:
    async with _admin(pool, redis_url) as (client, registry, _):
        await registry.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
        await registry.create_provider(ProviderKind.OPENAI_COMPATIBLE, "Local", "http://127.0.0.1:9/v1", None)
        page = await client.get("/ui/providers")
    assert "data-kind-select" in page.text
    assert re.search(r'value="openrouter" data-base-url="hidden" data-key-required="true"', page.text)
    assert re.search(r'value="ollama" data-base-url="optional" data-key-required="false"', page.text)
    assert re.search(r'value="azure" data-base-url="required" data-key-required="true"', page.text)
    assert "<script>" not in page.text
    # The built-in kind keeps its base URL override inside an advanced disclosure; a local server shows it.
    assert page.text.count('class="advanced"') == 1
    assert 'rel="icon" href="data:,"' in page.text


async def test_a_keyless_old_row_says_so_and_can_be_fixed_with_edit(pool: Pool, redis_url: str) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        provider_id = await _keyless(registry, ProviderKind.OPENROUTER, "OpenRouter")
        page = await client.get("/ui/providers")
        fixed = await client.post(
            f"/ui/providers/{provider_id}",
            data={
                "csrf_token": token,
                "name": "OpenRouter",
                "base_url": "",
                "api_key": API_KEY.get_secret_value(),
            },
            follow_redirects=False,
        )
        providers = await registry.list_providers()
    assert "No key yet" in page.text
    assert fixed.headers["location"] == "/ui/providers?notice=saved"
    assert providers[0].has_api_key


async def test_sync_failure_returns_to_providers_with_the_field_message(pool: Pool, redis_url: str) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        provider_id = await _keyless(registry, ProviderKind.OPENAI, "OpenAI")
        failed = await client.post(
            f"/ui/providers/{provider_id}/sync", data={"csrf_token": token}, follow_redirects=False
        )
        location = failed.headers["location"]
        page = await client.get(location)
    query = parse_qs(urlsplit(location).query)
    assert failed.status_code == 303
    assert urlsplit(location).path == "/ui/providers"
    assert (query["error"], query["field"], query["reason"]) == (
        ["VALIDATION_FAILED"],
        ["apiKey"],
        ["REQUIRED"],
    )
    assert query["rid"][0] == failed.headers["x-request-id"]
    assert "needs an API key" in page.text
    assert query["rid"][0] in page.text


async def test_sync_success_opens_that_providers_models_with_facts_and_a_count(
    pool: Pool, redis_url: str
) -> None:
    lister = FakeLister(
        [
            ListedModel("acme/thinker", _facts(Capability.CHAT, Capability.REASONING, context=200000)),
            ListedModel("acme/embed", _facts(Capability.EMBEDDING, context=8192)),
        ]
    )
    async with _admin(pool, redis_url, lister) as (client, registry, token):
        provider_id = await _keyless(registry, ProviderKind.OPENROUTER, "OpenRouter")
        synced = await client.post(
            f"/ui/providers/{provider_id}/sync", data={"csrf_token": token}, follow_redirects=False
        )
        page = await client.get(synced.headers["location"])
    assert synced.headers["location"] == f"/ui/models?provider={provider_id}&notice=synced&count=2"
    assert "Model list synced" in page.text
    assert 'class="tabular">2</span> models' in page.text
    assert MODEL_NAME.findall(page.text) == ["acme/embed", "acme/thinker"]
    assert "200000" in page.text
    assert "size is measured when the role is assigned" in page.text
    assert f'<option value="{provider_id}" selected>' in page.text


async def test_models_filter_by_capability_search_and_page(pool: Pool, redis_url: str) -> None:
    lister = FakeLister(
        [
            ListedModel("acme/chat-a", _facts(Capability.CHAT)),
            ListedModel("acme/chat-b", _facts(Capability.CHAT)),
            ListedModel("acme/embed-a", _facts(Capability.EMBEDDING)),
            ListedModel("acme/rerank-a", _facts(Capability.RERANK)),
        ]
    )
    async with _admin(pool, redis_url, lister, ui_models_page_size=2) as (client, registry, _):
        provider_id = await _keyless(registry, ProviderKind.OPENROUTER, "OpenRouter")
        await registry.sync_models(provider_id)
        everything = await client.get("/ui/models?provider=&capability=")
        embedding = await client.get("/ui/models?capability=embedding")
        rerank = await client.get(f"/ui/models?provider={provider_id}&capability=rerank")
        searched = await client.get("/ui/models?q=CHAT-b")
        second = await client.get("/ui/models?page=2")
        nonsense = await client.get("/ui/models?capability=<script>&provider=nope&page=-4")
        nothing = await client.get("/ui/models?q=zzz")
    assert everything.status_code == 200
    assert MODEL_NAME.findall(everything.text) == ["acme/chat-a", "acme/chat-b"]
    assert 'href="/ui/models?page=2"' in everything.text
    assert MODEL_NAME.findall(embedding.text) == ["acme/embed-a"]
    assert MODEL_NAME.findall(rerank.text) == ["acme/rerank-a"]
    assert MODEL_NAME.findall(searched.text) == ["acme/chat-b"]
    assert MODEL_NAME.findall(second.text) == ["acme/embed-a", "acme/rerank-a"]
    assert 'href="/ui/models"' in second.text
    assert nonsense.status_code == 200
    assert "<script>" not in nonsense.text
    assert "No model matches the filters" in nothing.text


async def test_a_refused_model_form_keeps_what_was_typed(pool: Pool, redis_url: str) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        provider = await registry.create_provider(ProviderKind.OLLAMA, "Ollama", "", None)
        other = await registry.create_provider(ProviderKind.OLLAMA, "Other", "", None)
        response = await client.post(
            "/ui/models",
            data={
                "csrf_token": token,
                "provider_id": str(other.id),
                "name": "  ",
                "capabilities": ["chat", "tools"],
            },
            follow_redirects=False,
        )
    assert response.status_code == 400
    assert "The name is not valid" in response.text
    assert f'<option value="{other.id}" selected>' in response.text
    assert f'<option value="{provider.id}" selected>' not in response.text
    assert 'value="chat" checked' in response.text
    assert 'value="tools" checked' in response.text
    assert 'value="vision" checked' not in response.text


async def test_the_model_forms_no_longer_ask_for_dims_or_context(pool: Pool, redis_url: str) -> None:
    async with _admin(pool, redis_url) as (client, registry, _):
        provider = await registry.create_provider(ProviderKind.OLLAMA, "Ollama", "", None)
        await registry.add_manual_model(provider.id, "chat-a", frozenset({Capability.CHAT}), None, None)
        page = await client.get("/ui/models")
    assert 'name="embedding_dims"' not in page.text
    assert 'name="context_window"' not in page.text


async def test_embedding_role_lists_a_model_without_dims_and_measures_it_on_save(
    pool: Pool, redis_url: str
) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        provider = await registry.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
        model = await registry.add_manual_model(
            provider.id, "acme/embed", frozenset({Capability.EMBEDDING}), None, None
        )
        roles = await client.get("/ui/roles")
        saved = await client.post(
            "/ui/roles",
            data={"csrf_token": token, "role": "embedding", "model_id": str(model.id)},
            follow_redirects=False,
        )
        assigned = (await registry.roles())[Role.EMBEDDING]
        after = await client.get("/ui/roles")
    assert "acme/embed" in roles.text.split('name="role" value="embedding"')[1].split("</form>")[0]
    assert saved.headers["location"] == "/ui/roles?notice=saved"
    assert assigned is not None
    assert assigned.embedding_dims == MEASURED_DIMS
    assert f"{MEASURED_DIMS} dims" in after.text


async def test_a_failed_probe_shows_the_provider_message_and_keeps_the_role(
    pool: Pool, redis_url: str
) -> None:
    checker = FakeChecker(error=AppError(ErrorCode.LLM_PROVIDER_REJECTED))
    async with _admin(pool, redis_url, checker=checker) as (client, registry, token):
        provider_id = await _keyless(registry, ProviderKind.OPENROUTER, "OpenRouter")
        model = await registry.add_manual_model(
            provider_id, "acme/embed", frozenset({Capability.EMBEDDING}), None, None
        )
        failed = await client.post(
            "/ui/roles",
            data={"csrf_token": token, "role": "embedding", "model_id": str(model.id)},
            follow_redirects=False,
        )
        page = await client.get(failed.headers["location"])
        assigned = (await registry.roles())[Role.EMBEDDING]
    assert parse_qs(urlsplit(failed.headers["location"]).query)["error"] == ["LLM_PROVIDER_REJECTED"]
    assert "The provider rejected the key or configuration." in page.text
    assert assigned is None


async def test_a_hostile_page_query_neither_raises_nor_builds_a_huge_number(
    pool: Pool, redis_url: str
) -> None:
    lister = FakeLister([ListedModel(f"acme/chat-{n}", _facts(Capability.CHAT)) for n in range(3)])
    async with _admin(pool, redis_url, lister, ui_models_page_size=2) as (client, registry, _):
        await registry.sync_models(await _keyless(registry, ProviderKind.OPENROUTER, "OpenRouter"))
        huge = await client.get("/ui/models?page=" + "9" * 5000)
        unicode_digit = await client.get("/ui/models?page=%C2%B2&count=%C2%B2")
        mixed = await client.get("/ui/models?page=1e9")
    assert huge.status_code == unicode_digit.status_code == mixed.status_code == 200
    assert MODEL_NAME.findall(huge.text) == ["acme/chat-2"]
    assert MODEL_NAME.findall(unicode_digit.text) == ["acme/chat-0", "acme/chat-1"]


async def test_the_edit_form_warns_that_a_new_address_removes_the_key(pool: Pool, redis_url: str) -> None:
    async with _admin(pool, redis_url) as (client, registry, token):
        provider = await registry.create_provider(ProviderKind.OPENROUTER, "OpenRouter", "", API_KEY)
        page = await client.get("/ui/providers")
        edited = await client.post(
            f"/ui/providers/{provider.id}",
            data={"csrf_token": token, "name": "OpenRouter", "base_url": "https://proxy.example/v1"},
            follow_redirects=False,
        )
        refused = await client.post(
            f"/ui/providers/{provider.id}",
            data={"csrf_token": token, "name": "OpenRouter", "base_url": "http://proxy.example/v1"},
            follow_redirects=False,
        )
        providers = await registry.list_providers()
    assert "removes this provider saved key" in page.text
    assert edited.headers["location"] == "/ui/providers?notice=saved"
    assert providers[0].has_api_key is False
    assert "field=baseUrl" in refused.headers["location"]
