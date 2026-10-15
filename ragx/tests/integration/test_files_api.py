import asyncio
import base64
import io
import json
import unicodedata
import uuid
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlsplit

import httpx
import pytest
from minio import Minio

from tests.integration.api_client import api_client, bearer, confirm, post_to_storage, presign, upload
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET
from thomas_ragx.registry.dev_seed import seed_dev_registry

pytestmark = pytest.mark.integration
ORG = "org-alpha"
REQUEST_ID = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
UPLOAD_CONTRACT = Path(__file__).resolve().parents[3] / "contracts" / "kb-upload.json"
UPLOAD_MAX_BYTES = json.loads(UPLOAD_CONTRACT.read_text(encoding="utf-8"))["maxFileBytes"]


@pytest.fixture
def minio(minio_config: dict[str, Any]) -> Minio:
    client = Minio(
        minio_config["endpoint"],
        access_key=minio_config["access_key"],
        secret_key=minio_config["secret_key"],
        secure=False,
    )
    for bucket in (RAW_BUCKET, PREVIEW_BUCKET):
        if not client.bucket_exists(bucket):
            client.make_bucket(bucket)
        for item in client.list_objects(bucket, recursive=True):
            client.remove_object(bucket, str(item.object_name))
    return client


async def _kb(client: httpx.AsyncClient, app: Any, org: str = ORG) -> str:
    await seed_dev_registry(app.state.registry_service)
    body = {"name": "Alpha", "externalParsing": True, "settingsVersion": 1}
    response = await client.put(f"/v1/orgs/{org}", json=body, headers=bearer(org, "org:provision"))
    return str(response.json()["data"]["defaultKbId"])


async def _count(pool: Pool, sql: bytes) -> int:
    async with pool.connection() as conn:
        row = await (await conn.execute(sql)).fetchone()
    assert row is not None
    return int(row["n"])


async def test_upload_list_download_round_trip(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        confirmed = await upload(client, ORG, kb_id, "notes.txt", b"hello ragx", REQUEST_ID)
        assert confirmed.status_code == 200, confirmed.text
        file = confirmed.json()["data"]
        listed = await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=bearer(ORG, "kb:read"))
        link = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files/{file['id']}/download-url", headers=bearer(ORG, "kb:read")
        )
    assert (file["status"], file["version"], file["contentType"], file["sizeBytes"]) == (
        "pending",
        1,
        "text/plain",
        10,
    )
    assert [f["id"] for f in listed.json()["data"]] == [file["id"]]
    assert listed.json()["meta"]["page"] == {"cursor": None, "limit": 20}
    async with httpx.AsyncClient() as storage:
        fetched = await storage.get(link.json()["data"]["url"])
    assert fetched.content == b"hello ragx"
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT stream, request_id, payload FROM outbox")).fetchone()
    assert row is not None
    assert (row["stream"], row["request_id"]) == ("thomas.ingest.requested", REQUEST_ID)
    assert row["payload"] == {"kb_id": kb_id, "file_id": file["id"], "version": 1}


async def test_presigned_post_accepts_exact_size_and_rejects_others(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        presigned = await presign(client, ORG, kb_id, "exact.txt", 10)
    assert presigned.status_code == 201, presigned.text
    ticket = presigned.json()["data"]
    assert ticket["upload"]["url"] == f"http://{minio_config['endpoint']}/kb-raw"
    too_big = await post_to_storage(ticket, b"x" * 11, "exact.txt")
    too_small = await post_to_storage(ticket, b"x" * 9, "exact.txt")
    exact = await post_to_storage(ticket, b"x" * 10, "exact.txt")
    # MinIO enforces the policy itself; core and ragx never see the bytes.
    assert (too_big.status_code, too_small.status_code, exact.status_code) == (400, 400, 204)
    policy = json.loads(base64.b64decode(ticket["upload"]["fields"]["policy"]))
    assert ["content-length-range", 10, 10] in policy["conditions"]
    assert ["eq", "$key", f"{ORG}/{kb_id}/{ticket['fileId']}"] in policy["conditions"]


async def test_confirm_rejects_missing_or_wrong_size_object(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        ticket = (await presign(client, ORG, kb_id, "m.txt", 5)).json()["data"]
        missing = await confirm(client, ORG, kb_id, ticket["fileId"])
        # The slot survives a MISSING answer, so the browser can POST again and confirm.
        assert (await post_to_storage(ticket, b"12345", "m.txt")).status_code == 204
        recovered = await confirm(client, ORG, kb_id, ticket["fileId"])
        wrong = (await presign(client, ORG, kb_id, "w.txt", 5)).json()["data"]
        key = f"{ORG}/{kb_id}/{wrong['fileId']}"
        minio.put_object(
            RAW_BUCKET, key, io.BytesIO(b"123"), 3
        )  # bypasses the policy, as a broken client could not
        mismatch = await confirm(client, ORG, kb_id, wrong["fileId"])
    assert missing.status_code == 400
    assert missing.json()["error"]["details"] == {"fields": [{"field": "file", "code": "MISSING"}]}
    assert recovered.status_code == 200
    assert mismatch.status_code == 400
    assert mismatch.json()["error"]["details"] == {"fields": [{"field": "file", "code": "SIZE_MISMATCH"}]}
    assert key not in {o.object_name for o in minio.list_objects(RAW_BUCKET, recursive=True)}
    assert await _count(pool, b"SELECT count(*) AS n FROM outbox") == 1


async def test_confirm_is_idempotent_and_dedupes_by_sha256(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await upload(client, ORG, kb_id, "a.md", b"same")
        again = await confirm(client, ORG, kb_id, first.json()["data"]["id"])
        copy = (await presign(client, ORG, kb_id, "copy.md", 4)).json()["data"]
        assert (await post_to_storage(copy, b"same", "copy.md")).status_code == 204
        folded = await confirm(client, ORG, kb_id, copy["fileId"])
        folded_again = await confirm(client, ORG, kb_id, copy["fileId"])
    original = first.json()["data"]
    assert {r.status_code for r in (first, again, folded, folded_again)} == {200}
    assert again.json()["data"] == original
    assert folded.json()["data"]["id"] == folded_again.json()["data"]["id"] == original["id"]
    assert [o.object_name for o in minio.list_objects(RAW_BUCKET, recursive=True)] == [
        f"{ORG}/{kb_id}/{original['id']}"
    ]
    assert await _count(pool, b"SELECT count(*) AS n FROM kb_files") == 1
    assert (
        await _count(pool, b"SELECT count(*) AS n FROM outbox WHERE stream = 'thomas.ingest.requested'") == 1
    )


async def test_concurrent_confirms_enqueue_one_ingest_job(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        ticket = (await presign(client, ORG, kb_id, "c.txt", 3)).json()["data"]
        assert (await post_to_storage(ticket, b"abc", "c.txt")).status_code == 204
        results = await asyncio.gather(*(confirm(client, ORG, kb_id, ticket["fileId"]) for _ in range(5)))
    assert {r.status_code for r in results} == {200}
    assert {r.json()["data"]["id"] for r in results} == {ticket["fileId"]}
    assert (
        await _count(pool, b"SELECT count(*) AS n FROM outbox WHERE stream = 'thomas.ingest.requested'") == 1
    )


async def test_concurrent_presigns_of_one_name_get_distinct_versions(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        presigned = await asyncio.gather(*(presign(client, ORG, kb_id, "same.txt", 2) for _ in range(3)))
        tickets = [p.json()["data"] for p in presigned]
        for index, ticket in enumerate(tickets):
            assert (await post_to_storage(ticket, f"x{index}".encode(), "same.txt")).status_code == 204
        confirmed = await asyncio.gather(*(confirm(client, ORG, kb_id, t["fileId"]) for t in tickets))
    assert {r.status_code for r in confirmed} == {200}
    assert sorted(r.json()["data"]["version"] for r in confirmed) == [1, 2, 3]


async def test_uploading_rows_are_hidden_until_confirmed(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await presign(client, ORG, kb_id, "h.txt", 3)).json()["data"]["fileId"]
        read, write = bearer(ORG, "kb:read"), bearer(ORG, "kb:write")
        listed = await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=read)
        kb = await client.get(f"/v1/knowledge-bases/{kb_id}", headers=read)
        base = f"/v1/knowledge-bases/{kb_id}/files/{file_id}"
        answers = [
            await client.post(f"{base}/retry", headers=write),
            await client.delete(base, headers=write),
            await client.get(f"{base}/download-url", headers=read),
        ]
    assert listed.json()["data"] == []
    assert kb.json()["data"]["fileCount"] == 0
    assert [a.json()["error"]["code"] for a in answers] == ["KB_FILE_NOT_FOUND"] * 3


async def test_download_url_forces_attachment_and_octet_stream_even_for_html(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await upload(client, ORG, kb_id, "page.html", b"<script>alert(1)</script>")).json()[
            "data"
        ]["id"]
        link = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}/download-url", headers=bearer(ORG, "kb:read")
        )
    url = link.json()["data"]["url"]
    assert urlsplit(url).netloc == minio_config["endpoint"], (
        "the browser downloads from MinIO's origin, never the app's"
    )
    async with httpx.AsyncClient() as storage:
        fetched = await storage.get(url)
    assert fetched.content == b"<script>alert(1)</script>"
    assert fetched.headers["content-type"] == "application/octet-stream"
    assert fetched.headers["content-disposition"] == "attachment; filename*=UTF-8''page.html"


async def test_same_content_returns_the_existing_file_and_same_name_gets_a_new_version(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await upload(client, ORG, kb_id, "a.md", b"one")
        again = await upload(client, ORG, kb_id, "copy.md", b"one")
        newer = await upload(client, ORG, kb_id, "a.md", b"two")
    assert (first.status_code, again.status_code, newer.status_code) == (200, 200, 200)
    assert again.json()["data"]["id"] == first.json()["data"]["id"]
    assert newer.json()["data"]["version"] == 2
    assert len(list(minio.list_objects(RAW_BUCKET, recursive=True))) == 2


async def test_upload_normalizes_vietnamese_names_and_downloads_with_rfc5987(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    expected = "attachment; filename*=UTF-8''T%C3%A0i%20li%E1%BB%87u.pdf"
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        decomposed = await upload(
            client, ORG, kb_id, unicodedata.normalize("NFD", "Tài liệu.pdf"), b"%PDF-1 a"
        )
        composed = await upload(client, ORG, kb_id, "Tài liệu.pdf", b"%PDF-1 b")
        file_id = decomposed.json()["data"]["id"]
        link = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}/download-url", headers=bearer(ORG, "kb:read")
        )
    assert decomposed.json()["data"]["name"] == "Tài liệu.pdf"
    assert composed.json()["data"]["version"] == 2
    url = link.json()["data"]["url"]
    assert parse_qs(urlsplit(url).query)["response-content-disposition"] == [expected]
    async with httpx.AsyncClient() as storage:
        fetched = await storage.get(url)
    assert fetched.headers["content-disposition"] == expected


async def test_presign_rejects_unsupported_type_size_and_bad_names(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    write = bearer(ORG, "kb:write")
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        url = f"/v1/knowledge-bases/{kb_id}/uploads"
        unsupported = await presign(client, ORG, kb_id, "setup.exe", 10)
        empty = await presign(client, ORG, kb_id, "a.pdf", 0)
        too_big = await presign(client, ORG, kb_id, "a.pdf", UPLOAD_MAX_BYTES + 1)
        bad_name = await presign(client, ORG, kb_id, "..", 10)
        extra = await client.post(url, json={"name": "a.pdf", "sizeBytes": 1, "kbId": kb_id}, headers=write)
        textual = await client.post(url, json={"name": "a.pdf", "sizeBytes": "10"}, headers=write)
    assert (unsupported.status_code, unsupported.json()["error"]["code"]) == (415, "UNSUPPORTED_MEDIA_TYPE")
    assert empty.status_code == 400
    assert empty.json()["error"]["details"] == {"fields": [{"field": "sizeBytes", "code": "EMPTY"}]}
    assert (too_big.status_code, too_big.json()["error"]["code"]) == (413, "PAYLOAD_TOO_LARGE")
    assert bad_name.status_code == 400
    assert bad_name.json()["error"]["details"] == {"fields": [{"field": "name", "code": "INVALID_NAME"}]}
    assert (extra.status_code, extra.json()["error"]["code"]) == (400, "VALIDATION_FAILED")
    assert textual.status_code == 400
    assert await _count(pool, b"SELECT count(*) AS n FROM kb_files") == 0


async def test_confirm_rejects_a_malformed_idempotency_key(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        ticket = (await presign(client, ORG, kb_id, "k.txt", 1)).json()["data"]
        headers = {**bearer(ORG, "kb:write"), "Idempotency-Key": "bad key"}
        rejected = await client.post(
            f"/v1/knowledge-bases/{kb_id}/files/{ticket['fileId']}/confirm", headers=headers
        )
    assert rejected.status_code == 400
    assert rejected.json()["error"]["details"] == {
        "fields": [{"field": "Idempotency-Key", "code": "INVALID"}]
    }


async def test_delete_retry_and_preview(pool: Pool, minio_config: dict[str, Any], minio: Minio) -> None:
    write, read = bearer(ORG, "kb:write"), bearer(ORG, "kb:read")
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await upload(client, ORG, kb_id, "r.txt", b"retry me")).json()["data"]["id"]
        pending_retry = await client.post(f"/v1/knowledge-bases/{kb_id}/files/{file_id}/retry", headers=write)
        async with pool.connection() as conn:
            await conn.execute(
                b"UPDATE kb_files SET status = 'failed', error_code = 'X' WHERE id = %s", (file_id,)
            )
        retried = await client.post(f"/v1/knowledge-bases/{kb_id}/files/{file_id}/retry", headers=write)
        preview = await client.get(f"/v1/knowledge-bases/{kb_id}/files/{file_id}/preview", headers=read)
        deleted = await client.delete(f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=write)
        listed = await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=read)
    assert pending_retry.json()["error"]["code"] == "CONFLICT"
    assert (retried.json()["data"]["status"], retried.json()["data"]["errorCode"]) == ("pending", "")
    assert preview.json()["error"]["code"] == "KB_FILE_NOT_FOUND"
    assert deleted.status_code == 204
    assert listed.json()["data"] == []
    async with pool.connection() as conn:
        streams = [
            row["stream"]
            for row in await (await conn.execute(b"SELECT stream FROM outbox ORDER BY created_at")).fetchall()
        ]
    assert streams == ["thomas.ingest.requested", "thomas.ingest.requested", "thomas.kb.purge.requested"]


async def test_file_count_ignores_deleted_files(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        kept = await upload(client, ORG, kb_id, "kept.txt", b"first file")
        dropped = await upload(client, ORG, kb_id, "dropped.txt", b"second file")
        assert (kept.status_code, dropped.status_code) == (200, 200)
        deleted = await client.delete(
            f"/v1/knowledge-bases/{kb_id}/files/{dropped.json()['data']['id']}",
            headers=bearer(ORG, "kb:write"),
        )
        assert deleted.status_code == 204, deleted.text
        fetched = await client.get(f"/v1/knowledge-bases/{kb_id}", headers=bearer(ORG, "kb:read"))
    assert fetched.json()["data"]["fileCount"] == 1


async def test_pagination_walks_every_file_once(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    read = bearer(ORG, "kb:read")
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        for index in range(5):
            await upload(client, ORG, kb_id, f"f{index}.txt", f"content {index}".encode())
        seen: list[str] = []
        cursor: str | None = None
        while True:
            params = {"limit": "2", **({"cursor": cursor} if cursor else {})}
            page = (
                await client.get(f"/v1/knowledge-bases/{kb_id}/files", params=params, headers=read)
            ).json()
            seen += [f["id"] for f in page["data"]]
            cursor = page["meta"]["page"]["cursor"]
            if cursor is None:
                break
        bad = await client.get(f"/v1/knowledge-bases/{kb_id}/files", params={"cursor": "%%%"}, headers=read)
        too_many = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files", params={"limit": "101"}, headers=read
        )
    assert len(seen) == len(set(seen)) == 5
    assert bad.json()["error"]["code"] == "VALIDATION_FAILED"
    assert too_many.json()["error"]["code"] == "VALIDATION_FAILED"


async def test_other_org_cannot_touch_files(pool: Pool, minio_config: dict[str, Any], minio: Minio) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await upload(client, ORG, kb_id, "s.txt", b"secret")).json()["data"]["id"]
        await _kb(client, app, "org-beta")
        beta_read, beta_write = bearer("org-beta", "kb:read"), bearer("org-beta", "kb:write")
        codes = [
            (await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=beta_read)).json()["error"][
                "code"
            ],
            (
                await client.get(
                    f"/v1/knowledge-bases/{kb_id}/files/{file_id}/download-url", headers=beta_read
                )
            ).json()["error"]["code"],
            (await client.delete(f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=beta_write)).json()[
                "error"
            ]["code"],
        ]
    assert codes == ["KB_NOT_FOUND"] * 3


async def test_other_org_cannot_presign_confirm_or_download(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        alpha_kb = await _kb(client, app)
        confirmed_id = (await upload(client, ORG, alpha_kb, "s.txt", b"secret")).json()["data"]["id"]
        slot_id = (await presign(client, ORG, alpha_kb, "slot.txt", 3)).json()["data"]["fileId"]
        beta_kb = await _kb(client, app, "org-beta")
        beta_read, beta_write = bearer("org-beta", "kb:read"), bearer("org-beta", "kb:write")
        alpha = f"/v1/knowledge-bases/{alpha_kb}"
        beta = f"/v1/knowledge-bases/{beta_kb}"
        answers = [
            await presign(client, "org-beta", alpha_kb, "x.txt", 1),
            await confirm(client, "org-beta", alpha_kb, confirmed_id),
            await confirm(client, "org-beta", alpha_kb, slot_id),
            await client.get(f"{alpha}/files/{confirmed_id}/download-url", headers=beta_read),
            await client.get(f"{beta}/files/{confirmed_id}/download-url", headers=beta_read),
            await client.post(f"{beta}/files/{confirmed_id}/confirm", headers=beta_write),
        ]
    assert [a.json()["error"]["code"] for a in answers] == ["KB_NOT_FOUND"] * 4 + ["KB_FILE_NOT_FOUND"] * 2


async def test_head_on_download_url_and_list_answers_like_get_with_no_body(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    read = bearer(ORG, "kb:read")
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await upload(client, ORG, kb_id, "h.txt", b"head")).json()["data"]["id"]
        for path in (
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}/download-url",
            f"/v1/knowledge-bases/{kb_id}/files",
        ):
            got, head = await client.get(path, headers=read), await client.head(path, headers=read)
            assert (head.status_code, head.content) == (got.status_code, b""), path
            assert head.headers["content-type"] == got.headers["content-type"], path
        missing = await client.head(
            f"/v1/knowledge-bases/{kb_id}/files/{uuid.uuid4()}/download-url", headers=read
        )
    assert missing.status_code == 404


async def test_confirm_of_duplicate_content_survives_a_failed_object_delete(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await upload(client, ORG, kb_id, "a.txt", b"same")
        storage = app.state.file_service._storage

        async def refuse(_: str, __: str) -> None:
            raise RuntimeError("storage hiccup")

        storage.remove = refuse  # the sweeper cleans the orphan later (ruling A17)
        second = await upload(client, ORG, kb_id, "b.txt", b"same")
    assert second.status_code == 200
    assert second.json()["data"]["id"] == first.json()["data"]["id"]


async def test_preview_head_answers_from_stat_without_reading(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await upload(client, ORG, kb_id, "p.txt", b"preview")).json()["data"]["id"]
        minio.put_object(PREVIEW_BUCKET, f"{ORG}/{kb_id}/{file_id}.json", io.BytesIO(b'{"blocks":[]}'), 13)

        async def no_reads(_: str, __: str) -> None:
            raise AssertionError("HEAD must not open the object")

        app.state.file_service._storage.stream_object = no_reads
        head = await client.head(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}/preview", headers=bearer(ORG, "kb:read")
        )
    assert head.status_code == 200
    assert head.headers["content-length"] == "13"
    assert head.content == b""
