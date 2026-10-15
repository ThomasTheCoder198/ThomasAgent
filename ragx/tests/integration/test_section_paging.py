"""kb_read_section reads a long section a page at a time and stops at its character cap (Codex I2)."""

import uuid
from types import SimpleNamespace
from typing import Any
from unittest.mock import AsyncMock

import pytest
from minio import Minio
from opentelemetry.sdk.trace import TracerProvider
from qdrant_client import AsyncQdrantClient

from tests.integration.ingest_support import ORG, REQUEST_ID, pipeline_for, uploaded_file
from thomas_ragx.auth.caller import Caller
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.retrieval import tool_repository
from thomas_ragx.retrieval.tool_service import KbToolService

pytestmark = pytest.mark.integration
PARAGRAPHS = 300
PAGE_ROWS = 7
SECTION_MAX_CHARS = 2000
TOOL = "kb_read_section"


def _document() -> bytes:
    paragraphs = [
        f"Đoạn {i} nói về chính sách nghỉ phép và các điều kiện áp dụng." for i in range(PARAGRAPHS)
    ]
    half = PARAGRAPHS // 2
    body = "\n\n".join(paragraphs[:half]) + "\n\n## Phụ lục\n\n" + "\n\n".join(paragraphs[half:])
    return f"# Mở đầu\n\nTrước mục lớn.\n\n# Mục lớn\n\n{body}\n\n# Mục sau\n\nSau mục lớn.\n".encode()


def _capped(rows: list[dict[str, Any]], cap: int) -> tuple[list[dict[str, Any]], bool]:
    """The pre-I2 algorithm over the whole section: the output must not change."""
    kept: list[dict[str, Any]] = []
    used = 0
    for row in rows:
        remaining = cap - used
        if len(row["text"]) > remaining:
            if not kept:
                kept.append({**row, "text": row["text"][:remaining]})
            return kept, True
        kept.append(row)
        used += len(row["text"])
    return kept, False


async def _section_rows(pool: Pool, node_id: uuid.UUID) -> list[dict[str, Any]]:
    async with pool.connection() as conn:
        rows = await (
            await conn.execute(
                b"WITH RECURSIVE tree AS (SELECT id FROM doc_nodes WHERE id = %s "
                b"UNION ALL SELECT n.id FROM doc_nodes n JOIN tree t ON n.parent_id = t.id) "
                b"SELECT block_id, block_type, text, locator FROM doc_blocks "
                b"WHERE node_id IN (SELECT id FROM tree) ORDER BY position",
                (node_id,),
            )
        ).fetchall()
    return [dict(row) for row in rows]


@pytest.mark.parametrize("cap", [SECTION_MAX_CHARS, 10_000_000])
async def test_read_section_pages_through_the_section_and_stops_at_the_cap(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Any,
    monkeypatch: pytest.MonkeyPatch,
    cap: int,
) -> None:
    kb_id, file_id = await uploaded_file(pool, minio_config, "dai.md", _document())
    await pipeline_for(pool, minio, qdrant, tmp_path).run(
        IngestJob(ORG, uuid.UUID(kb_id), uuid.UUID(file_id), 1, REQUEST_ID)
    )
    async with pool.connection() as conn:
        node = await (
            await conn.execute(
                b"SELECT id FROM doc_nodes WHERE file_id = %s AND title = %s", (file_id, "Mục lớn")
            )
        ).fetchone()
    assert node is not None
    whole = await _section_rows(pool, node["id"])
    assert len(whole) > PARAGRAPHS  # both halves, the sub-heading and the nested section's blocks
    pages: list[int] = []
    section_blocks = tool_repository.section_blocks

    async def counted(*args: Any, **kwargs: Any) -> Any:
        page = await section_blocks(*args, **kwargs)
        pages.append(len(page))
        return page

    monkeypatch.setattr(tool_repository, "section_blocks", counted)
    service = KbToolService(
        pool,
        SimpleNamespace(search=AsyncMock()),  # type: ignore[arg-type]
        TracerProvider().get_tracer(__name__),
        timeout_seconds=20,
        section_max_chars=cap,
        section_page_rows=PAGE_ROWS,
        outline_max_nodes=500,
        page_default_limit=20,
        page_max_limit=100,
    )
    section = await service.read_section(
        Caller(ORG, f"tool:{TOOL}", kb_id, REQUEST_ID, "jti"), TOOL, node["id"]
    )
    expected, truncated = _capped(whole, cap)
    strip = ("block_id", "block_type", "text", "locator")
    assert [{k: b[k] for k in strip} for b in section["blocks"]] == expected
    assert section["truncated"] is truncated
    fetched = sum(pages)
    if truncated:
        assert fetched <= len(expected) + PAGE_ROWS < len(whole)
    else:
        assert fetched == len(whole) and all(size <= PAGE_ROWS for size in pages)
