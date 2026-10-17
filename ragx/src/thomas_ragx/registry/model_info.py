from collections.abc import Mapping
from decimal import Decimal
from typing import Any, Final

from thomas_ragx.registry.model import EMPTY_FACTS, Capability, ModelFacts, ProviderKind

LITELLM_PROVIDER_NAMES: Final[Mapping[ProviderKind, str]] = {
    ProviderKind.OPENROUTER: "openrouter",
    ProviderKind.OPENAI: "openai",
    ProviderKind.ANTHROPIC: "anthropic",
    ProviderKind.GOOGLE: "gemini",
    ProviderKind.AZURE: "azure",
    ProviderKind.OLLAMA: "ollama",
    # LiteLLM routes any OpenAI-compatible server through its openai provider plus api_base.
    ProviderKind.OPENAI_COMPATIBLE: "openai",
    ProviderKind.COHERE: "cohere",
    ProviderKind.VOYAGE: "voyage",
    ProviderKind.JINA: "jina_ai",
}
MODE_CAPABILITIES: Final[Mapping[str, Capability]] = {
    "chat": Capability.CHAT,
    "embedding": Capability.EMBEDDING,
    "rerank": Capability.RERANK,
}
FLAG_CAPABILITIES: Final[Mapping[str, Capability]] = {
    "supports_vision": Capability.VISION,
    "supports_function_calling": Capability.TOOLS,
    "supports_reasoning": Capability.REASONING,
}


def litellm_model_name(kind: ProviderKind, name: str) -> str:
    return f"{LITELLM_PROVIDER_NAMES[kind]}/{name}"


def _decimal(value: Any) -> Decimal | None:
    return Decimal(str(value)) if isinstance(value, int | float) else None


def _integer(value: Any) -> int | None:
    return value if isinstance(value, int) else None


def facts_from_info(info: Mapping[str, Any]) -> ModelFacts:
    """Turn one entry of LiteLLM's bundled model map into registry facts."""
    capabilities = {capability for flag, capability in FLAG_CAPABILITIES.items() if info.get(flag)}
    mode_capability = MODE_CAPABILITIES.get(str(info.get("mode")))
    if mode_capability is not None:
        capabilities.add(mode_capability)
    dims = _integer(info.get("output_vector_size")) if Capability.EMBEDDING in capabilities else None
    return ModelFacts(
        capabilities=frozenset(capabilities),
        embedding_dims=dims,
        context_window=_integer(info.get("max_input_tokens")),
        input_cost_per_token=_decimal(info.get("input_cost_per_token")),
        output_cost_per_token=_decimal(info.get("output_cost_per_token")),
    )


def describe_model(kind: ProviderKind, name: str) -> ModelFacts:
    """Read bundled facts; leave unknown models for the operator to describe."""
    from litellm import get_model_info

    try:
        info: Mapping[str, Any] = dict(
            get_model_info(model=name, custom_llm_provider=LITELLM_PROVIDER_NAMES[kind])
        )
    except Exception:
        return EMPTY_FACTS
    return facts_from_info(info)
