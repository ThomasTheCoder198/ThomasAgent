from decimal import Decimal
from typing import Any

import httpx
import pytest
from pydantic import SecretStr

from thomas_ragx.registry.model import Capability, ModelFacts, ProviderAccess, ProviderKind
from thomas_ragx.registry.openrouter_listing import OpenRouterLister

KEY = "sk-or-v1-unit-test-key"
CHAT_BODY: dict[str, Any] = {
    "data": [
        {
            "id": "acme/vision-thinker",
            "context_length": 200000,
            "architecture": {"input_modalities": ["text", "image"], "output_modalities": ["text"]},
            "supported_parameters": ["tools", "reasoning", "temperature"],
            "pricing": {"prompt": "0.000003", "completion": "0.000015"},
        },
        {
            "id": "acme/plain-chat",
            "context_length": 8192,
            "architecture": {"input_modalities": ["text"], "output_modalities": ["text"]},
            "supported_parameters": ["include_reasoning"],
            "pricing": {"prompt": "-1", "completion": "-1"},
        },
        {
            "id": "acme/painter",
            "architecture": {"input_modalities": ["text"], "output_modalities": ["image"]},
            "supported_parameters": [],
        },
        {"id": ""},
        {"name": "no id"},
    ]
}
EMBEDDING_BODY: dict[str, Any] = {
    "data": [
        {
            "id": "acme/embed-large",
            "context_length": 8192,
            "architecture": {"input_modalities": ["text", "image"], "output_modalities": ["embeddings"]},
            "supported_parameters": ["tools"],
            "pricing": {"prompt": "0.0000001", "completion": "0"},
        }
    ]
}
MODELS_PATH = "/api/v1/models"
EMBEDDINGS_PATH = "/api/v1/embeddings/models"


class Server:
    def __init__(self, chat: Any = None, embedding: Any = None, status: int = 200) -> None:
        self.requests: list[httpx.Request] = []
        self._bodies = {MODELS_PATH: chat or CHAT_BODY, EMBEDDINGS_PATH: embedding or EMBEDDING_BODY}
        self._status = status

    def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        return httpx.Response(self._status, json=self._bodies[request.url.path])


def _lister(server: Server) -> OpenRouterLister:
    return OpenRouterLister(timeout_seconds=5.0, transport=httpx.MockTransport(server))


def _access(key: str | None = None, base_url: str = "") -> ProviderAccess:
    return ProviderAccess(ProviderKind.OPENROUTER, base_url, SecretStr(key) if key else None)


async def _facts(server: Server, access: ProviderAccess) -> dict[str, ModelFacts]:
    listed = await _lister(server).list_models(access)
    assert all(model.facts is not None for model in listed)
    return {model.name: model.facts for model in listed if model.facts}


async def test_chat_facts_map_from_modalities_and_parameters() -> None:
    facts = (await _facts(Server(), _access()))["acme/vision-thinker"]
    assert facts.capabilities == {Capability.CHAT, Capability.TOOLS, Capability.REASONING, Capability.VISION}
    assert facts.context_window == 200000
    assert facts.input_cost_per_token == Decimal("0.000003")
    assert facts.output_cost_per_token == Decimal("0.000015")
    assert facts.embedding_dims is None


async def test_include_reasoning_alone_is_not_reasoning_and_negative_prices_are_unknown() -> None:
    # Core maps only `reasoning`; the two registries must agree.
    facts = (await _facts(Server(), _access()))["acme/plain-chat"]
    assert facts.capabilities == {Capability.CHAT}
    assert facts.input_cost_per_token is None
    assert facts.output_cost_per_token is None


async def test_image_only_output_is_not_chat_and_rows_without_an_id_are_skipped() -> None:
    models = await _facts(Server(), _access())
    assert models["acme/painter"].capabilities == frozenset()
    assert models["acme/painter"].context_window is None
    assert set(models) == {"acme/vision-thinker", "acme/plain-chat", "acme/painter", "acme/embed-large"}


async def test_embedding_models_come_from_the_embeddings_list_with_only_that_capability() -> None:
    facts = (await _facts(Server(), _access()))["acme/embed-large"]
    assert facts.capabilities == {Capability.EMBEDDING}
    assert facts.context_window == 8192


async def test_an_id_on_both_lists_keeps_the_union() -> None:
    both = {"data": [{"id": "acme/both", "architecture": {"output_modalities": ["text"]}}]}
    embedding = {"data": [{"id": "acme/both", "architecture": {"output_modalities": ["embeddings"]}}]}
    facts = (await _facts(Server(chat=both, embedding=embedding), _access()))["acme/both"]
    assert facts.capabilities == {Capability.CHAT, Capability.EMBEDDING}


async def test_both_lists_are_read_and_nothing_is_sent_without_a_key() -> None:
    server = Server()
    await _lister(server).list_models(_access())
    assert sorted(request.url.path for request in server.requests) == [EMBEDDINGS_PATH, MODELS_PATH]
    assert all("authorization" not in request.headers for request in server.requests)
    assert all("x-request-id" not in request.headers for request in server.requests)


async def test_the_key_is_sent_as_a_bearer_token_when_the_provider_has_one() -> None:
    server = Server()
    await _lister(server).list_models(_access(KEY))
    assert {request.headers["authorization"] for request in server.requests} == {f"Bearer {KEY}"}


async def test_a_custom_base_url_replaces_the_default_host() -> None:
    server = Server()
    await _lister(server).list_models(_access(base_url="https://proxy.example/api/v1/"))
    assert {request.url.host for request in server.requests} == {"proxy.example"}


async def test_an_http_error_propagates_for_the_boundary_to_map() -> None:
    with pytest.raises(httpx.HTTPStatusError):
        await _lister(Server(status=500)).list_models(_access())
