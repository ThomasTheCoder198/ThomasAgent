from collections.abc import AsyncIterator
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import httpx
import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient

from tests.integration.api_client import api_client
from tests.integration.search_support import (
    IndexedCorpus,
    assign_fake_rerank_model,
    call_tool,
    indexed_corpus,
    registry_for,
    tool_service_for,
)
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ERROR_DEFINITIONS, ErrorCode
from thomas_ragx.retrieval.qdrant_store import FIELD_FILE, any_condition, tenant_filter

pytestmark = pytest.mark.integration
ORG_A = "org-a"
ORG_B = "org-b"
SHARED_FILE = "quy-che-nhan-su.md"
SHARED_NODE_TITLE = "Nghỉ phép năm"
QUERY = "nghỉ phép năm"
OTHER_KB_NAME = "Kho khác"

TOOLS = {
    "kb_search": lambda ids: {"query": QUERY},
    "kb_outline": lambda ids: {"fileId": ids["file"]},
    "kb_read_section": lambda ids: {"nodeId": ids["node"]},
    "kb_list_documents": lambda ids: {},
}


@dataclass(frozen=True)
class OrgFixture:
    """One org's indexed corpus plus the ids of the same document and section in it."""

    corpus: IndexedCorpus
    ids: dict[str, str]

    @property
    def kb_id(self) -> str:
        return str(self.corpus.kb_id)

    @property
    def file_ids(self) -> set[str]:
        return {str(file_id) for file_id in self.corpus.file_ids.values()}


async def _org(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Path,
    org_id: str,
    kb_name: str | None = None,
) -> OrgFixture:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, org_id, kb_name)
    file_id = corpus.file_ids[SHARED_FILE]
    async with pool.connection() as conn:
        node = await (
            await conn.execute(
                b"SELECT id FROM doc_nodes WHERE file_id = %s AND title = %s", (file_id, SHARED_NODE_TITLE)
            )
        ).fetchone()
    assert node is not None
    return OrgFixture(corpus, {"file": str(file_id), "node": str(node["id"])})


@pytest.fixture
async def two_orgs(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    qdrant_url: str,
    tmp_path: Path,
) -> AsyncIterator[tuple[httpx.AsyncClient, OrgFixture, OrgFixture]]:
    # Both orgs index the same files, so any leak would return plausible, identical-looking content.
    a = await _org(pool, minio_config, minio, qdrant, tmp_path, ORG_A)
    b = await _org(pool, minio_config, minio, qdrant, tmp_path, ORG_B)
    assert a.file_ids.isdisjoint(b.file_ids) and a.ids["node"] != b.ids["node"]
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    service = tool_service_for(pool, qdrant, tmp_path, registry)
    async with api_client(
        pool, minio_config=minio_config, qdrant_url=qdrant_url, tool_service_override=service
    ) as (client, _):
        yield client, a, b


async def test_tools_never_cross_orgs(two_orgs) -> None:  # type: ignore[no-untyped-def]
    client, a, b = two_orgs
    found = (await call_tool(client, "kb_search", {"query": QUERY}, a.kb_id, ORG_A)).json()["data"]
    assert found["results"] and {r["fileId"] for r in found["results"]} <= a.file_ids
    for tool, body in TOOLS.items():
        crossed = await call_tool(client, tool, body(b.ids), b.kb_id, ORG_A)  # A's token, B's KB claim
        assert crossed.status_code == 404 and crossed.json()["error"]["code"] == "KB_NOT_FOUND", tool
    own_kb_other_ids = {
        "kb_outline": ("KB_FILE_NOT_FOUND", {"fileId": b.ids["file"]}),
        "kb_read_section": ("KB_NODE_NOT_FOUND", {"nodeId": b.ids["node"]}),
    }
    for tool, (code, body) in own_kb_other_ids.items():
        answer = await call_tool(client, tool, body, a.kb_id, ORG_A)  # A's KB, B's ids in the body
        assert answer.status_code == 404 and answer.json()["error"]["code"] == code, tool
    own_filter = {"query": QUERY, "filters": {"fileIds": [a.ids["file"]]}}
    own = await call_tool(client, "kb_search", own_filter, a.kb_id, ORG_A)  # positive control
    assert own.status_code == 200 and own.json()["data"]["results"]
    foreign_filter = {"query": QUERY, "filters": {"fileIds": [b.ids["file"]]}}
    filtered = await call_tool(client, "kb_search", foreign_filter, a.kb_id, ORG_A)
    assert filtered.status_code == 200 and filtered.json()["data"]["results"] == []
    listed = await call_tool(client, "kb_list_documents", {}, a.kb_id, ORG_A)
    documents = listed.json()["data"]
    assert listed.status_code == 200 and documents
    assert {d["fileId"] for d in documents} <= a.file_ids


@pytest.mark.parametrize("smuggled", ["kbId", "orgId", "tenantId", "kb_id"])
async def test_tool_body_cannot_name_a_kb_or_org(two_orgs, smuggled: str) -> None:  # type: ignore[no-untyped-def]
    client, a, b = two_orgs
    answer = await call_tool(client, "kb_search", {"query": "x", smuggled: b.kb_id}, a.kb_id, ORG_A)
    assert answer.status_code == 400 and answer.json()["error"]["code"] == "VALIDATION_FAILED"


async def test_a_token_for_an_unprovisioned_org_is_refused(two_orgs) -> None:  # type: ignore[no-untyped-def]
    client, a, _ = two_orgs
    answer = await call_tool(client, "kb_search", {"query": "x"}, a.kb_id, "org-never")
    assert answer.status_code == ERROR_DEFINITIONS[ErrorCode.ORG_NOT_PROVISIONED].http_status
    assert answer.json()["error"]["code"] == "ORG_NOT_PROVISIONED"


@pytest.fixture
async def two_kbs(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    qdrant_url: str,
    tmp_path: Path,
) -> AsyncIterator[tuple[httpx.AsyncClient, OrgFixture, OrgFixture]]:
    # Same org, same files, same embedding model: only the kb_id filter keeps the two KBs apart.
    main = await _org(pool, minio_config, minio, qdrant, tmp_path, ORG_A)
    other = await _org(pool, minio_config, minio, qdrant, tmp_path, ORG_A, OTHER_KB_NAME)
    assert main.kb_id != other.kb_id and main.file_ids.isdisjoint(other.file_ids)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    service = tool_service_for(pool, qdrant, tmp_path, registry)
    async with api_client(
        pool, minio_config=minio_config, qdrant_url=qdrant_url, tool_service_override=service
    ) as (client, _):
        yield client, main, other


async def test_tools_never_cross_kbs_within_one_org(  # type: ignore[no-untyped-def]
    two_kbs, pool: Pool, qdrant: AsyncQdrantClient
) -> None:
    client, main, other = two_kbs
    found = (await call_tool(client, "kb_search", {"query": QUERY}, main.kb_id, ORG_A)).json()["data"]
    assert found["results"] and {r["fileId"] for r in found["results"]} <= main.file_ids
    other_ids = {
        "kb_outline": ("KB_FILE_NOT_FOUND", {"fileId": other.ids["file"]}),
        "kb_read_section": ("KB_NODE_NOT_FOUND", {"nodeId": other.ids["node"]}),
    }
    for tool, (code, body) in other_ids.items():
        answer = await call_tool(client, tool, body, main.kb_id, ORG_A)  # main KB claim, other KB's ids
        assert answer.status_code == 404 and answer.json()["error"]["code"] == code, tool
        own = await call_tool(client, tool, body, other.kb_id, ORG_A)  # positive control
        assert own.status_code == 200, tool
    foreign_filter = {"query": QUERY, "filters": {"fileIds": [other.ids["file"]]}}
    filtered = await call_tool(client, "kb_search", foreign_filter, main.kb_id, ORG_A)
    assert filtered.status_code == 200 and filtered.json()["data"]["results"] == []
    listed = await call_tool(client, "kb_list_documents", {}, main.kb_id, ORG_A)
    documents = listed.json()["data"]
    assert listed.status_code == 200 and documents
    assert {d["fileId"] for d in documents} <= main.file_ids
    # Qdrant itself: one shared collection, and the tenant filter of one KB never matches the other's points.
    async with pool.connection() as conn:
        rows = await (
            await conn.execute(
                b"SELECT collection_name FROM knowledge_bases WHERE id = ANY(%s)",
                ([main.corpus.kb_id, other.corpus.kb_id],),
            )
        ).fetchall()
    collections = {row["collection_name"] for row in rows}
    assert len(collections) == 1
    collection = collections.pop()
    other_files = any_condition(FIELD_FILE, sorted(other.file_ids))
    leaked = await qdrant.count(
        collection, count_filter=tenant_filter(ORG_A, main.kb_id, other_files), exact=True
    )
    assert leaked.count == 0
    present = await qdrant.count(
        collection, count_filter=tenant_filter(ORG_A, other.kb_id, other_files), exact=True
    )
    assert present.count > 0  # positive control: the other KB's points exist in that collection
