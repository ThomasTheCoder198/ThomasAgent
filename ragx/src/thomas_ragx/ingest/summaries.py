from collections.abc import Awaitable, Callable, Sequence
from dataclasses import dataclass
from typing import Final
from uuid import UUID

import structlog

from thomas_ragx.providers.chat import ChatModel
from thomas_ragx.registry.model import ModelHandle

# Same language as the section; one or two sentences; used by kb_outline (M1.3c).
SUMMARY_PROMPT: Final = (
    "Tóm tắt mục tài liệu sau trong 1-2 câu, cùng ngôn ngữ với nội dung, không thêm thông tin mới.\n\n"
)
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class SectionText:
    node_id: UUID
    text: str


class NodeSummarizer:
    """Optional doc_nodes summaries; provider failures never fail ingest (ruling B14)."""

    def __init__(
        self,
        chat: ChatModel,
        resolve_model: Callable[[], Awaitable[ModelHandle]],
        *,
        enabled: bool,
        max_nodes: int,
        input_chars: int,
        max_tokens: int,
    ) -> None:
        self._chat = chat
        self._resolve = resolve_model
        self._enabled = enabled
        self._max_nodes = max_nodes
        self._input_chars = input_chars
        self._max_tokens = max_tokens

    async def summarize(self, sections: Sequence[SectionText]) -> dict[UUID, str]:
        if not self._enabled or not sections:
            return {}
        try:
            model = await self._resolve()
        except Exception as exc:  # no chat.fast role: summaries are simply absent
            log.info("section summaries skipped", error_type=type(exc).__name__)
            return {}
        summaries: dict[UUID, str] = {}
        for section in sections[: self._max_nodes]:
            prompt = SUMMARY_PROMPT + section.text[: self._input_chars]
            try:
                summaries[section.node_id] = await self._chat.complete(model, prompt, self._max_tokens)
            except Exception as exc:
                log.warning("section summary failed", error_type=type(exc).__name__)
        return summaries
