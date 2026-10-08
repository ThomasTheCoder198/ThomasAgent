from collections.abc import Mapping
from dataclasses import dataclass
from decimal import Decimal
from enum import StrEnum
from typing import Final
from uuid import UUID

from pydantic import SecretStr


class ProviderKind(StrEnum):
    OPENROUTER = "openrouter"
    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    GOOGLE = "google"
    AZURE = "azure"
    OLLAMA = "ollama"
    OPENAI_COMPATIBLE = "openai_compatible"
    COHERE = "cohere"
    VOYAGE = "voyage"
    JINA = "jina"


class Capability(StrEnum):
    CHAT = "chat"
    TOOLS = "tools"
    VISION = "vision"
    REASONING = "reasoning"
    EMBEDDING = "embedding"
    RERANK = "rerank"
    DECISION = "decision"


class Role(StrEnum):
    EMBEDDING = "embedding"
    VISION = "vision"
    CHAT_FAST = "chat.fast"
    RERANK = "rerank"
    DECISION = "decision"


class ModelSource(StrEnum):
    SYNC = "sync"
    MANUAL = "manual"


# A role accepts any listed capability; a decision model can also rerank (main spec §3.3).
ROLE_CAPABILITIES: Final[Mapping[Role, frozenset[Capability]]] = {
    Role.EMBEDDING: frozenset({Capability.EMBEDDING}),
    Role.VISION: frozenset({Capability.VISION}),
    Role.CHAT_FAST: frozenset({Capability.CHAT}),
    Role.RERANK: frozenset({Capability.RERANK, Capability.DECISION}),
    Role.DECISION: frozenset({Capability.DECISION}),
}


@dataclass(frozen=True)
class Provider:
    id: UUID
    kind: ProviderKind
    name: str
    base_url: str
    has_api_key: bool


@dataclass(frozen=True)
class ModelFacts:
    capabilities: frozenset[Capability]
    embedding_dims: int | None
    context_window: int | None
    input_cost_per_token: Decimal | None
    output_cost_per_token: Decimal | None


EMPTY_FACTS: Final = ModelFacts(frozenset(), None, None, None, None)


@dataclass(frozen=True)
class ModelRecord:
    id: UUID
    provider_id: UUID
    name: str
    capabilities: frozenset[Capability]
    capabilities_edited: bool
    embedding_dims: int | None
    context_window: int | None
    input_cost_per_token: Decimal | None
    output_cost_per_token: Decimal | None
    source: ModelSource


@dataclass(frozen=True)
class ProviderAccess:
    """Decrypted access for one call; SecretStr keeps the key out of reprs and logs."""

    kind: ProviderKind
    base_url: str
    api_key: SecretStr | None
