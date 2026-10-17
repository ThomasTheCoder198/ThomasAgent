import asyncio
from collections.abc import Mapping
from decimal import Decimal, InvalidOperation
from time import perf_counter
from typing import Any, Final

import httpx
import structlog

from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.registry.listing import LISTING_ROUTES, ListedModel
from thomas_ragx.registry.model import Capability, ModelFacts, ProviderAccess, ProviderKind

CHAT_MODELS_PATH: Final = "/models"
EMBEDDING_MODELS_PATH: Final = "/embeddings/models"
AUTHORIZATION_HEADER: Final = "Authorization"
MODALITY_TEXT: Final = "text"
MODALITY_IMAGE: Final = "image"
MODALITY_EMBEDDINGS: Final = "embeddings"
PARAM_TOOLS: Final = "tools"
PARAM_REASONING: Final = "reasoning"
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
log = structlog.get_logger(__name__)


def _strings(value: Any) -> frozenset[str]:
    return (
        frozenset(item for item in value if isinstance(item, str)) if isinstance(value, list) else frozenset()
    )


def _positive_int(value: Any) -> int | None:
    return value if isinstance(value, int) and not isinstance(value, bool) and value > 0 else None


def _per_token(value: Any) -> Decimal | None:
    """OpenRouter prices are strings per token; negative prices mark routers with no fixed price."""
    try:
        price = Decimal(str(value))
    except InvalidOperation:
        return None
    return price if price.is_finite() and price >= 0 else None


def _capabilities(item: Mapping[str, Any]) -> frozenset[Capability]:
    architecture = item.get("architecture")
    architecture = architecture if isinstance(architecture, Mapping) else {}
    outputs = _strings(architecture.get("output_modalities"))
    if MODALITY_EMBEDDINGS in outputs:
        # An embedding model's tools, reasoning or image input say nothing about chat or the vision role.
        return frozenset({Capability.EMBEDDING})
    parameters = _strings(item.get("supported_parameters"))
    capabilities: set[Capability] = set()
    if MODALITY_TEXT in outputs:
        capabilities.add(Capability.CHAT)
    if PARAM_TOOLS in parameters:
        capabilities.add(Capability.TOOLS)
    if PARAM_REASONING in parameters:
        capabilities.add(Capability.REASONING)
    if MODALITY_IMAGE in _strings(architecture.get("input_modalities")):
        capabilities.add(Capability.VISION)
    return frozenset(capabilities)


def _listed_model(item: Any) -> ListedModel | None:
    if not isinstance(item, Mapping) or not isinstance(item.get("id"), str) or not item["id"]:
        return None
    pricing = item.get("pricing")
    pricing = pricing if isinstance(pricing, Mapping) else {}
    facts = ModelFacts(
        capabilities=_capabilities(item),
        embedding_dims=None,  # not in the list; measured when the embedding role is assigned
        context_window=_positive_int(item.get("context_length")),
        input_cost_per_token=_per_token(pricing.get("prompt")),
        output_cost_per_token=_per_token(pricing.get("completion")),
    )
    return ListedModel(item["id"], facts)


def _merge(first: ListedModel, second: ListedModel) -> ListedModel:
    """An id on both lists keeps what either list knows."""
    assert first.facts is not None and second.facts is not None  # noqa: S101 - built by _listed_model
    a, b = first.facts, second.facts
    facts = ModelFacts(
        capabilities=a.capabilities | b.capabilities,
        embedding_dims=a.embedding_dims or b.embedding_dims,
        context_window=a.context_window or b.context_window,
        input_cost_per_token=a.input_cost_per_token or b.input_cost_per_token,
        output_cost_per_token=a.output_cost_per_token or b.output_cost_per_token,
    )
    return ListedModel(first.name, facts)


class OpenRouterLister:
    """Reads both public lists; sends the key only when the provider has one and never the request id."""

    def __init__(self, timeout_seconds: float, transport: httpx.AsyncBaseTransport | None = None) -> None:
        self._timeout_seconds = timeout_seconds
        self._transport = transport

    async def _fetch(
        self, client: httpx.AsyncClient, base_url: str, path: str, headers: Mapping[str, str]
    ) -> list[ListedModel]:
        started = perf_counter()
        outcome = OUTCOME_ERROR
        status_code = 0
        # A failure propagates to ModelListing, which logs it once; headers never reach a log line.
        try:
            response = await client.get(f"{base_url}{path}", headers=headers)
            status_code = response.status_code
            response.raise_for_status()
            data = response.json().get("data", [])
            listed = [model for model in map(_listed_model, data) if model is not None]
            outcome = OUTCOME_SUCCESS
            return listed
        finally:
            log.info(
                "model listing call completed",
                kind=ProviderKind.OPENROUTER.value,
                path=path,
                status=status_code,
                outcome=outcome,
                duration_ms=elapsed_ms(started),
            )

    async def list_models(self, access: ProviderAccess) -> list[ListedModel]:
        base_url = (access.base_url or LISTING_ROUTES[access.kind].default_base_url).rstrip("/")
        headers = (
            {AUTHORIZATION_HEADER: f"Bearer {access.api_key.get_secret_value()}"} if access.api_key else {}
        )
        async with httpx.AsyncClient(timeout=self._timeout_seconds, transport=self._transport) as client:
            chat, embedding = await asyncio.gather(
                self._fetch(client, base_url, CHAT_MODELS_PATH, headers),
                self._fetch(client, base_url, EMBEDDING_MODELS_PATH, headers),
            )
        merged: dict[str, ListedModel] = {}
        for model in [*chat, *embedding]:
            known = merged.get(model.name)
            merged[model.name] = _merge(known, model) if known else model
        return list(merged.values())
