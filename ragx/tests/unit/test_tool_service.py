import asyncio
import unicodedata
from contextlib import asynccontextmanager
from types import SimpleNamespace
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import StatusCode

from thomas_ragx.auth.caller import Caller
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.retrieval import tool_repository
from thomas_ragx.retrieval.search_service import SearchFilters
from thomas_ragx.retrieval.tool_service import KbToolService


@asynccontextmanager
async def _connection():  # type: ignore[no-untyped-def]
    yield None


def _service(timeout: float = 1):  # type: ignore[no-untyped-def]
    provider = TracerProvider()
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    service = KbToolService(
        SimpleNamespace(connection=_connection),
        SimpleNamespace(search=AsyncMock()),
        provider.get_tracer(__name__),
        timeout_seconds=timeout,
        section_max_chars=4,
        section_page_rows=64,
        outline_max_nodes=3,
        page_default_limit=20,
        page_max_limit=100,
    )
    service._kb = AsyncMock(return_value=SimpleNamespace(id=uuid4()))
    return service, exporter


async def test_section_character_cap_includes_an_oversized_first_block(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    service, _ = _service()
    node_id, file_id = uuid4(), uuid4()
    monkeypatch.setattr(
        tool_repository, "node_in_kb", AsyncMock(return_value={"id": node_id, "file_id": file_id})
    )
    monkeypatch.setattr(tool_repository, "node_path", AsyncMock(return_value=[]))
    monkeypatch.setattr(
        tool_repository,
        "section_blocks",
        AsyncMock(
            return_value=[
                {"block_id": "first", "text": "abcdef", "locator": {"kind": "text_lines"}},
            ]
        ),
    )
    section = await service.read_section(
        Caller("org", "tool:kb_read_section", str(uuid4()), "req_test", "jti"), "kb_read_section", node_id
    )
    assert sum(len(block["text"]) for block in section["blocks"]) <= 4
    assert section["truncated"] is True


async def test_tool_deadline_maps_to_catalog_timeout_without_exception_content() -> None:
    service, exporter = _service(timeout=0.001)

    async def slow(*args):  # type: ignore[no-untyped-def]
        await asyncio.sleep(1)

    service._search.search = slow
    with pytest.raises(AppError) as caught:
        await service.search(
            Caller("org", "tool:kb_search", str(uuid4()), "req_test", "jti"),
            "kb_search",
            "sensitive query",
            SearchFilters(),
        )
    assert caught.value.code is ErrorCode.DEPENDENCY_TIMEOUT
    span = exporter.get_finished_spans()[0]
    assert span.status.is_ok is False and span.status.description is None
    assert not span.events
    assert "sensitive query" not in str(span.attributes)


async def test_name_filter_is_nfc_normalised_like_stored_names(monkeypatch: pytest.MonkeyPatch) -> None:
    service, _ = _service()
    listed = AsyncMock(return_value=[])
    monkeypatch.setattr(tool_repository, "list_documents", listed)
    nfd = unicodedata.normalize("NFD", "Lõi lọc")
    assert nfd != unicodedata.normalize("NFC", nfd)
    await service.list_documents(
        Caller("org", "tool:kb_list_documents", str(uuid4()), "req_test", "jti"),
        "kb_list_documents",
        nfd,
        None,
        None,
        None,
    )
    assert listed.await_args.args[3] == unicodedata.normalize("NFC", nfd)


async def test_client_errors_do_not_mark_the_tool_span_as_failed(monkeypatch: pytest.MonkeyPatch) -> None:
    service, exporter = _service()
    monkeypatch.setattr(tool_repository, "node_in_kb", AsyncMock(return_value=None))
    with pytest.raises(AppError) as caught:
        await service.read_section(
            Caller("org", "tool:kb_read_section", str(uuid4()), "req_test", "jti"),
            "kb_read_section",
            uuid4(),
        )
    assert caught.value.code is ErrorCode.KB_NODE_NOT_FOUND
    span = exporter.get_finished_spans()[0]
    assert span.status.status_code is StatusCode.UNSET
    assert not span.events


async def test_server_side_app_errors_mark_the_tool_span_as_failed(monkeypatch: pytest.MonkeyPatch) -> None:
    service, exporter = _service()
    monkeypatch.setattr(
        tool_repository, "node_in_kb", AsyncMock(side_effect=AppError(ErrorCode.INTERNAL_ERROR))
    )
    with pytest.raises(AppError):
        await service.read_section(
            Caller("org", "tool:kb_read_section", str(uuid4()), "req_test", "jti"),
            "kb_read_section",
            uuid4(),
        )
    assert exporter.get_finished_spans()[0].status.status_code is StatusCode.ERROR


async def test_unexpected_errors_mark_the_tool_span_as_failed_without_their_message(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    service, exporter = _service()
    monkeypatch.setattr(tool_repository, "node_in_kb", AsyncMock(side_effect=RuntimeError("secret detail")))
    with pytest.raises(RuntimeError):
        await service.read_section(
            Caller("org", "tool:kb_read_section", str(uuid4()), "req_test", "jti"),
            "kb_read_section",
            uuid4(),
        )
    span = exporter.get_finished_spans()[0]
    assert span.status.status_code is StatusCode.ERROR and span.status.description is None
    assert not span.events and "secret detail" not in str(span.attributes)


async def test_section_read_stops_fetching_once_the_character_cap_is_reached(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    service, _ = _service()
    service._section_max_chars = 250
    node_id, file_id = uuid4(), uuid4()
    rows = [
        {"block_id": f"b{i}", "position": i, "text": "x" * 100, "locator": {"kind": "text_lines"}}
        for i in range(1000)
    ]
    served: list[int] = []

    async def section_blocks(*_: object, after_position: int | None = None, limit: int | None = None) -> list:
        start = 0 if after_position is None else after_position + 1
        page = rows[start:] if limit is None else rows[start : start + limit]
        served.append(len(page))
        return page

    monkeypatch.setattr(
        tool_repository, "node_in_kb", AsyncMock(return_value={"id": node_id, "file_id": file_id})
    )
    monkeypatch.setattr(tool_repository, "node_path", AsyncMock(return_value=[]))
    monkeypatch.setattr(tool_repository, "section_blocks", section_blocks)
    section = await service.read_section(
        Caller("org", "tool:kb_read_section", str(uuid4()), "req_test", "jti"), "kb_read_section", node_id
    )
    assert [block["block_id"] for block in section["blocks"]] == ["b0", "b1"]
    assert section["truncated"] is True
    assert sum(served) < len(rows)  # the rest of the section is never read
    assert sum(served) <= len(section["blocks"]) + service._section_page_rows
