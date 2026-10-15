from collections.abc import AsyncIterator
from pathlib import Path
from types import SimpleNamespace
from typing import Any
from unittest.mock import AsyncMock

import pytest
from minio import Minio
from opentelemetry.sdk.trace import TracerProvider
from qdrant_client import AsyncQdrantClient

from tests.integration.api_client import api_client, bearer
from tests.integration.search_support import (
    assign_fake_rerank_model,
    call_tool,
    indexed_corpus,
    registry_for,
    tool_service_for,
)
from thomas_ragx.db.pool import Pool
from thomas_ragx.retrieval.tool_service import KbToolService

pytestmark = pytest.mark.integration
ORG = "org-tools"


async def _call(client, tool: str, body: dict, kb_id: str | None, org: str = ORG):  # type: ignore[no-untyped-def]
    return await call_tool(client, tool, body, kb_id, org)


async def test_kb_search_returns_labelled_blocks(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    answer = await _call(client, "kb_search", {"query": "Lõi lọc bảo hành mấy tháng?"}, str(corpus.kb_id))
    assert answer.status_code == 200, answer.text
    data = answer.json()["data"]
    assert data["insufficientEvidence"] is False and 1 <= len(data["results"]) <= 8
    block = data["results"][0]["blocks"][0]
    assert block["label"] == "[1.1]" and block["locator"]["kind"] in {"text_lines", "sheet_range"}


async def test_outline_and_read_section_walk_a_document(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    file_id = str(corpus.file_ids["quy-che-nhan-su.md"])
    outline = (await _call(client, "kb_outline", {"fileId": file_id}, str(corpus.kb_id))).json()["data"]
    titles = [n["title"] for n in outline["nodes"]]
    assert titles[:3] == ["quy-che-nhan-su.md", "Quy chế nhân sự", "Giờ làm việc"]
    leave = next(n for n in outline["nodes"] if n["title"] == "Nghỉ phép năm")
    section = (await _call(client, "kb_read_section", {"nodeId": leave["nodeId"]}, str(corpus.kb_id))).json()[
        "data"
    ]
    assert section["path"] == ["Quy chế nhân sự", "Nghỉ phép năm"] and section["truncated"] is False
    assert any("12 ngày nghỉ phép năm" in b["text"] for b in section["blocks"])


async def test_list_documents_filters_by_name_and_type(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    listed = await _call(
        client, "kb_list_documents", {"nameContains": "GIA-DICH", "docType": "csv"}, str(corpus.kb_id)
    )
    assert [d["name"] for d in listed.json()["data"]] == ["bang-gia-dich-vu.csv"]
    assert listed.json()["meta"]["page"]["limit"] == 20


async def test_tool_routes_need_their_own_scope_and_a_kb_claim(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    wrong_scope = await client.post(
        "/v1/tools/kb_search", json={"query": "x"}, headers=bearer(ORG, "kb:read", str(corpus.kb_id))
    )
    other_tool = await client.post(
        "/v1/tools/kb_search", json={"query": "x"}, headers=bearer(ORG, "tool:kb_outline", str(corpus.kb_id))
    )
    no_kb = await _call(client, "kb_search", {"query": "x"}, None)
    assert [r.json()["error"]["code"] for r in (wrong_scope, other_tool, no_kb)] == ["FORBIDDEN"] * 3


async def test_manifest_route_serves_the_tools(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, _ = tool_client
    manifest = (await client.get("/v1/manifest", headers=bearer(ORG, "kb:read"))).json()["data"]
    assert len(manifest["tools"]) == 4


async def test_search_doc_type_filter_ignores_case_and_leading_dot(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    for doc_types in (["MD"], [".md"], ["PDF", ".Md"]):
        answer = await _call(
            client,
            "kb_search",
            {"query": "Nghỉ phép năm", "filters": {"docTypes": doc_types}},
            str(corpus.kb_id),
        )
        results = answer.json()["data"]["results"]
        assert answer.status_code == 200 and results, doc_types
        assert {r["fileName"].rsplit(".", 1)[-1] for r in results} == {"md"}


async def test_search_doc_type_filter_rejects_non_alphanumeric_entries(tool_client) -> None:  # type: ignore[no-untyped-def]
    client, corpus = tool_client
    answer = await _call(
        client, "kb_search", {"query": "x", "filters": {"docTypes": ["p.df"]}}, str(corpus.kb_id)
    )
    assert answer.status_code == 400 and answer.json()["error"]["code"] == "VALIDATION_FAILED"


async def test_tool_deadline_is_http_504_with_a_masked_body(  # type: ignore[no-untyped-def]
    pool: Pool,
    minio_config: dict[str, Any],
    qdrant_url: str,
    tool_client,
) -> None:
    _, corpus = tool_client
    expired = KbToolService(
        pool,
        SimpleNamespace(search=AsyncMock()),  # type: ignore[arg-type]
        TracerProvider().get_tracer(__name__),
        timeout_seconds=0.000001,
        section_max_chars=8000,
        outline_max_nodes=500,
        page_default_limit=20,
        page_max_limit=100,
    )
    async with api_client(
        pool, minio_config=minio_config, qdrant_url=qdrant_url, tool_service_override=expired
    ) as (client, _):
        answer = await _call(client, "kb_list_documents", {}, str(corpus.kb_id))
    assert answer.status_code == 504
    assert answer.json()["error"]["code"] == "INTERNAL_ERROR"


@pytest.fixture
async def tool_client(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    qdrant_url: str,
    tmp_path: Path,
) -> AsyncIterator[tuple[Any, Any]]:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    service = tool_service_for(pool, qdrant, tmp_path, registry)
    async with api_client(
        pool, minio_config=minio_config, qdrant_url=qdrant_url, tool_service_override=service
    ) as (client, _):
        yield client, corpus
