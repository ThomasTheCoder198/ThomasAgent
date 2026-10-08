from thomas_ragx.registry.model import Capability, ProviderKind
from thomas_ragx.registry.model_info import describe_model, litellm_model_name


def test_known_embedding_model_reports_capability_and_dims() -> None:
    facts = describe_model(ProviderKind.OPENAI, "text-embedding-3-small")
    assert Capability.EMBEDDING in facts.capabilities
    assert facts.embedding_dims == 1536


def test_unknown_model_has_empty_facts() -> None:
    facts = describe_model(ProviderKind.OPENAI_COMPATIBLE, "no-such-model-xyz")
    assert facts.capabilities == frozenset()
    assert facts.embedding_dims is None


def test_litellm_model_name_prefixes_the_litellm_provider() -> None:
    assert (
        litellm_model_name(ProviderKind.OPENROUTER, "openai/gpt-4o-mini") == "openrouter/openai/gpt-4o-mini"
    )
    assert litellm_model_name(ProviderKind.OPENAI_COMPATIBLE, "local-embed") == "openai/local-embed"
