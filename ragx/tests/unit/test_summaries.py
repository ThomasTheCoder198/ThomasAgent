import uuid
from unittest.mock import AsyncMock

from tests.rag_fakes import FakeChat
from thomas_ragx.ingest.summaries import NodeSummarizer, SectionText
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import ModelHandle


async def _model() -> ModelHandle:
    return object()  # type: ignore[return-value]  # FakeChat never reads it


async def _unassigned() -> ModelHandle:
    raise AppError(ErrorCode.LLM_ROLE_NOT_ASSIGNED)


def _sections(count: int) -> list[SectionText]:
    return [SectionText(uuid.uuid4(), f"Nội dung mục {n} " * 50) for n in range(count)]


async def test_summaries_are_capped_truncated_and_keyed_by_node() -> None:
    chat = FakeChat()
    sections = _sections(3)
    summarizer = NodeSummarizer(chat, _model, enabled=True, max_nodes=2, input_chars=100, max_tokens=50)
    summaries = await summarizer.summarize(sections)
    assert list(summaries) == [sections[0].node_id, sections[1].node_id]
    assert all(len(p) < 100 + 400 for p in chat.prompts)  # the instruction plus at most input_chars of text


async def test_summaries_skip_when_disabled_or_the_role_is_missing() -> None:
    chat = FakeChat()
    off = NodeSummarizer(chat, _model, enabled=False, max_nodes=5, input_chars=100, max_tokens=50)
    missing = NodeSummarizer(chat, _unassigned, enabled=True, max_nodes=5, input_chars=100, max_tokens=50)
    assert await off.summarize(_sections(2)) == {} and await missing.summarize(_sections(2)) == {}
    assert chat.prompts == []


async def test_a_failing_summary_never_fails_ingest() -> None:
    sections = _sections(2)
    chat = FakeChat(fail_on="mục 0")
    summaries = await NodeSummarizer(
        chat, _model, enabled=True, max_nodes=5, input_chars=1000, max_tokens=50
    ).summarize(sections)
    assert list(summaries) == [sections[1].node_id]


async def test_empty_sections_do_not_resolve_the_model() -> None:
    unexpected = AsyncMock()

    chat = FakeChat()
    summarizer = NodeSummarizer(chat, unexpected, enabled=True, max_nodes=5, input_chars=100, max_tokens=50)
    assert await summarizer.summarize([]) == {}
    unexpected.assert_not_awaited()
    assert chat.prompts == []


async def test_registry_outage_never_fails_ingest() -> None:
    async def down() -> ModelHandle:
        raise RuntimeError("registry unavailable")

    chat = FakeChat()
    summarizer = NodeSummarizer(chat, down, enabled=True, max_nodes=5, input_chars=100, max_tokens=50)
    assert await summarizer.summarize(_sections(1)) == {}
    assert chat.prompts == []
