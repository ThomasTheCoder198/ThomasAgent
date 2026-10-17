import os

import pytest

from thomas_ragx.platform.litellm_policy import apply_litellm_policy
from thomas_ragx.registry.catalog_listing import CatalogLister
from thomas_ragx.registry.model import Capability, ModelFacts, ProviderAccess, ProviderKind
from thomas_ragx.registry.model_info import litellm_model_name


async def _catalog(kind: ProviderKind) -> dict[str, ModelFacts]:
    apply_litellm_policy(os.environ)
    listed = await CatalogLister().list_models(ProviderAccess(kind, "", None))
    assert all(model.facts is not None for model in listed)
    return {model.name: model.facts for model in listed if model.facts}


async def test_cohere_catalog_has_rerank_and_embedding_without_the_provider_prefix() -> None:
    models = await _catalog(ProviderKind.COHERE)
    assert models["rerank-v3.5"].capabilities == {Capability.RERANK}
    assert models["embed-v4.0"].capabilities == {Capability.EMBEDDING}
    assert not any(name.startswith("cohere/") for name in models)


async def test_cohere_completion_and_ocr_modes_are_not_registry_models() -> None:
    models = await _catalog(ProviderKind.COHERE)
    assert "command" not in models
    assert "parse-v5.0" not in models


async def test_voyage_catalog_names_are_stored_without_the_prefix_litellm_adds_back() -> None:
    models = await _catalog(ProviderKind.VOYAGE)
    assert models["rerank-2.5"].capabilities == {Capability.RERANK}
    assert models["voyage-3"].capabilities == {Capability.EMBEDDING}
    assert litellm_model_name(ProviderKind.VOYAGE, "voyage-3") == "voyage/voyage-3"
    assert not any("/" in name for name in models)


async def test_jina_catalog_has_its_reranker() -> None:
    models = await _catalog(ProviderKind.JINA)
    assert models["jina-reranker-v2-base-multilingual"].capabilities == {Capability.RERANK}


@pytest.mark.parametrize("kind", [ProviderKind.COHERE, ProviderKind.VOYAGE, ProviderKind.JINA])
async def test_every_catalog_has_a_rerank_model_for_the_mandatory_rerank_role(kind: ProviderKind) -> None:
    models = await _catalog(kind)
    assert any(Capability.RERANK in facts.capabilities for facts in models.values())


async def test_known_embedding_dims_travel_with_the_catalog() -> None:
    assert (await _catalog(ProviderKind.COHERE))["embed-v4.0"].embedding_dims == 1536
