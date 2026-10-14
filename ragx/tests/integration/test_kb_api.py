import asyncio

import pytest

from tests.integration.api_client import api_client, bearer
from thomas_ragx.db.pool import Pool
from thomas_ragx.registry.dev_seed import seed_dev_registry

pytestmark = pytest.mark.integration
PROVISION = {"name": "Alpha Co", "externalParsing": True}


async def _provision(client, app, org_id: str) -> dict[str, object]:  # type: ignore[no-untyped-def]
    await seed_dev_registry(app.state.registry_service)
    response = await client.put(f"/v1/orgs/{org_id}", json=PROVISION, headers=bearer(org_id, "org:provision"))
    assert response.status_code == 200, response.text
    return response.json()["data"]


async def test_provision_is_idempotent_and_creates_one_default_kb(pool: Pool) -> None:
    async with api_client(pool) as (client, app):
        first = await _provision(client, app, "org-alpha")
        second = await _provision(client, app, "org-alpha")
        assert first["defaultKbId"] == second["defaultKbId"]
        listed = await client.get("/v1/knowledge-bases", headers=bearer("org-alpha", "kb:read"))
    kbs = listed.json()["data"]
    assert [kb["isDefault"] for kb in kbs] == [True]
    assert kbs[0]["embeddingDims"] == 1536
    assert kbs[0]["fileCount"] == 0


async def test_concurrent_first_provisions_create_one_default_kb(pool: Pool) -> None:
    async with api_client(pool) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        responses = await asyncio.gather(
            *(
                client.put("/v1/orgs/org-race", json=PROVISION, headers=bearer("org-race", "org:provision"))
                for _ in range(2)
            )
        )
    assert [r.status_code for r in responses] == [200, 200]
    assert len({r.json()["data"]["defaultKbId"] for r in responses}) == 1


async def test_provision_without_embedding_role_creates_nothing(pool: Pool) -> None:
    async with api_client(pool) as (client, _):
        response = await client.put(
            "/v1/orgs/org-new", json=PROVISION, headers=bearer("org-new", "org:provision")
        )
        listed = await client.get("/v1/knowledge-bases", headers=bearer("org-new", "kb:read"))
    assert response.json()["error"]["code"] == "LLM_ROLE_NOT_ASSIGNED"
    assert listed.json()["error"]["code"] == "ORG_NOT_PROVISIONED"


async def test_provision_path_must_match_the_token_org(pool: Pool) -> None:
    async with api_client(pool) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        response = await client.put(
            "/v1/orgs/org-beta", json=PROVISION, headers=bearer("org-alpha", "org:provision")
        )
    assert response.status_code == 403
    assert response.json()["error"]["code"] == "FORBIDDEN"


async def test_kb_crud_and_default_kb_rules(pool: Pool) -> None:
    write, read = bearer("org-alpha", "kb:write"), bearer("org-alpha", "kb:read")
    async with api_client(pool) as (client, app):
        org = await _provision(client, app, "org-alpha")
        created = await client.post("/v1/knowledge-bases", json={"name": "  Chính sách  "}, headers=write)
        assert created.status_code == 201
        kb_id = created.json()["data"]["id"]
        assert created.json()["data"]["name"] == "Chính sách"
        duplicate = await client.post("/v1/knowledge-bases", json={"name": "Chính sách"}, headers=write)
        assert duplicate.json()["error"]["code"] == "CONFLICT"
        renamed = await client.patch(
            f"/v1/knowledge-bases/{kb_id}", json={"name": "Quy trình"}, headers=write
        )
        assert renamed.json()["data"]["name"] == "Quy trình"
        default_delete = await client.delete(f"/v1/knowledge-bases/{org['defaultKbId']}", headers=write)
        assert default_delete.json()["error"]["code"] == "CONFLICT"
        assert (await client.delete(f"/v1/knowledge-bases/{kb_id}", headers=write)).status_code == 204
        gone = await client.get(f"/v1/knowledge-bases/{kb_id}", headers=read)
        assert gone.json()["error"]["code"] == "KB_NOT_FOUND"
        recreated = await client.post("/v1/knowledge-bases", json={"name": "Quy trình"}, headers=write)
        assert recreated.status_code == 201
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT stream, payload FROM outbox")).fetchone()
    assert row is not None
    assert row["stream"] == "thomas.kb.purge.requested"
    assert row["payload"] == {"kind": "kb", "kb_id": kb_id}


async def test_org_a_token_cannot_reach_org_b_kb(pool: Pool) -> None:
    async with api_client(pool) as (client, app):
        await _provision(client, app, "org-alpha")
        beta = await _provision(client, app, "org-beta")
        beta_kb = beta["defaultKbId"]
        read, write = bearer("org-alpha", "kb:read"), bearer("org-alpha", "kb:write")
        responses = [
            await client.get(f"/v1/knowledge-bases/{beta_kb}", headers=read),
            await client.patch(f"/v1/knowledge-bases/{beta_kb}", json={"name": "x"}, headers=write),
            await client.delete(f"/v1/knowledge-bases/{beta_kb}", headers=write),
        ]
        listed = await client.get("/v1/knowledge-bases", headers=read)
        smuggled = await client.post(
            "/v1/knowledge-bases", json={"name": "x", "orgId": "org-beta"}, headers=write
        )
    assert [r.json()["error"]["code"] for r in responses] == ["KB_NOT_FOUND"] * 3
    assert beta_kb not in {kb["id"] for kb in listed.json()["data"]}
    assert smuggled.json()["error"]["code"] == "VALIDATION_FAILED"


async def test_manifest_lists_the_four_kb_tools(pool: Pool) -> None:
    async with api_client(pool) as (client, _):
        response = await client.get("/v1/manifest", headers=bearer("org-alpha", "kb:read"))
    assert [tool["name"] for tool in response.json()["data"]["tools"]] == [
        "kb_search",
        "kb_outline",
        "kb_read_section",
        "kb_list_documents",
    ]
    assert response.json()["data"]["name"] == "ragx"


async def test_new_knowledge_base_records_its_collection_name(pool: Pool) -> None:
    async with api_client(pool) as (client, app):
        model = await seed_dev_registry(app.state.registry_service)
        await _provision(client, app, "org-c")
        created = await client.post(
            "/v1/knowledge-bases", json={"name": "Second"}, headers=bearer("org-c", "kb:write")
        )
    assert created.status_code == 201
    async with pool.connection() as conn:
        row = await (
            await conn.execute(
                b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (created.json()["data"]["id"],)
            )
        ).fetchone()
    assert row is not None and row["collection_name"] == f"kb_{model.id.hex}_{model.embedding_dims}"
