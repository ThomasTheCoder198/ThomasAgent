import asyncio
import unicodedata
from typing import Any

import httpx
import pytest
from minio import Minio

from tests.integration.api_client import api_client, bearer
from tests.tokens import mint
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import file_repository
from thomas_ragx.kb.storage import PREVIEW_BUCKET, RAW_BUCKET
from thomas_ragx.registry.dev_seed import seed_dev_registry

pytestmark = pytest.mark.integration
ORG = "org-alpha"
REQUEST_ID = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"


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
    body = {"name": "Alpha", "externalParsing": True}
    response = await client.put(f"/v1/orgs/{org}", json=body, headers=bearer(org, "org:provision"))
    return str(response.json()["data"]["defaultKbId"])


async def _upload(
    client: httpx.AsyncClient, kb_id: str, name: str, content: bytes, **headers: str
) -> httpx.Response:
    auth = bearer(ORG, "kb:write")
    if request_id := headers.get("X-Request-Id"):
        auth = {
            "Authorization": f"Bearer {mint(ORG, 'kb:write', rid=request_id)}",
            "X-Request-Id": request_id,
        }
    return await client.post(
        f"/v1/knowledge-bases/{kb_id}/files",
        files={"file": (name, content, "application/octet-stream")},
        headers={**auth, **headers},
    )


async def test_upload_list_download_round_trip(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        uploaded = await _upload(client, kb_id, "notes.txt", b"hello ragx", **{"X-Request-Id": REQUEST_ID})
        assert uploaded.status_code == 201, uploaded.text
        file = uploaded.json()["data"]
        assert (file["status"], file["version"], file["contentType"], file["sizeBytes"]) == (
            "pending",
            1,
            "text/plain",
            10,
        )
        listed = await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=bearer(ORG, "kb:read"))
        assert [f["id"] for f in listed.json()["data"]] == [file["id"]]
        assert listed.json()["meta"]["page"] == {"cursor": None, "limit": 20}
        raw = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files/{file['id']}/raw", headers=bearer(ORG, "kb:read")
        )
        assert raw.content == b"hello ragx"
        assert raw.headers["content-type"].startswith("text/plain")
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT stream, payload, request_id FROM outbox")).fetchone()
    assert row is not None
    assert (row["stream"], row["request_id"]) == ("thomas.ingest.requested", REQUEST_ID)
    assert row["payload"] == {"kb_id": kb_id, "file_id": file["id"], "version": 1}


async def test_same_content_returns_the_existing_file_and_same_name_gets_a_new_version(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await _upload(client, kb_id, "a.md", b"one")
        again = await _upload(client, kb_id, "copy.md", b"one")
        newer = await _upload(client, kb_id, "a.md", b"two")
    assert (first.status_code, again.status_code, newer.status_code) == (201, 200, 201)
    assert again.json()["data"]["id"] == first.json()["data"]["id"]
    assert newer.json()["data"]["version"] == 2
    assert len(list(minio.list_objects(RAW_BUCKET, recursive=True))) == 2


async def test_upload_normalizes_vietnamese_names_and_downloads_with_rfc5987(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        decomposed = await _upload(client, kb_id, unicodedata.normalize("NFD", "Tài liệu.pdf"), b"%PDF-1 a")
        composed = await _upload(client, kb_id, "Tài liệu.pdf", b"%PDF-1 b")
        file_id = decomposed.json()["data"]["id"]
        raw = await client.get(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}/raw", headers=bearer(ORG, "kb:read")
        )
    assert decomposed.json()["data"]["name"] == "Tài liệu.pdf"
    assert composed.json()["data"]["version"] == 2
    assert raw.headers["content-disposition"] == "attachment; filename*=UTF-8''T%C3%A0i%20li%E1%BB%87u.pdf"


async def test_concurrent_uploads_never_fail_on_unique_constraints(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        renamed = await asyncio.gather(
            _upload(client, kb_id, "same.txt", b"x1"), _upload(client, kb_id, "same.txt", b"x2")
        )
        duplicated = await asyncio.gather(
            _upload(client, kb_id, "d.txt", b"dup"), _upload(client, kb_id, "e.txt", b"dup")
        )
    assert sorted(r.status_code for r in renamed) == [201, 201]
    assert sorted(r.json()["data"]["version"] for r in renamed) == [1, 2]
    assert sorted(r.status_code for r in duplicated) == [200, 201]
    assert len({r.json()["data"]["id"] for r in duplicated}) == 1


async def test_idempotency_key_replays_the_first_result(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await _upload(client, kb_id, "k.txt", b"first", **{"Idempotency-Key": "upload-key-0001"})
        retry = await _upload(client, kb_id, "k.txt", b"second", **{"Idempotency-Key": "upload-key-0001"})
        invalid = await _upload(client, kb_id, "k.txt", b"third", **{"Idempotency-Key": "bad key"})
    assert retry.status_code == 200
    assert retry.json()["data"]["id"] == first.json()["data"]["id"]
    assert invalid.json()["error"]["code"] == "VALIDATION_FAILED"


async def test_idempotency_key_of_a_deleted_file_uploads_again(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    key = "upload-key-deleted"
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        first = await _upload(client, kb_id, "gone.txt", b"gone", **{"Idempotency-Key": key})
        file_id = first.json()["data"]["id"]
        deleted = await client.delete(
            f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=bearer(ORG, "kb:write")
        )
        again = await _upload(client, kb_id, "gone.txt", b"gone", **{"Idempotency-Key": key})
        replay = await _upload(client, kb_id, "gone.txt", b"gone", **{"Idempotency-Key": key})
    assert (first.status_code, deleted.status_code, again.status_code) == (201, 204, 201)
    assert again.json()["data"]["id"] != file_id
    # The key now belongs to the new file, so a retry replays it instead of failing.
    assert (replay.status_code, replay.json()["data"]["id"]) == (200, again.json()["data"]["id"])


async def test_idempotency_key_binds_a_duplicate_content_result(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    key = "upload-key-duplicate"
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        original = await _upload(client, kb_id, "original.txt", b"same")
        duplicate = await _upload(client, kb_id, "duplicate.txt", b"same", **{"Idempotency-Key": key})
        replay = await _upload(client, kb_id, "different.txt", b"different", **{"Idempotency-Key": key})
    assert (original.status_code, duplicate.status_code, replay.status_code) == (201, 200, 200)
    assert duplicate.json()["data"]["id"] == replay.json()["data"]["id"] == original.json()["data"]["id"]
    assert len(list(minio.list_objects(RAW_BUCKET, recursive=True))) == 1


async def test_idempotency_key_binds_duplicate_found_under_kb_lock(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, monkeypatch: pytest.MonkeyPatch
) -> None:
    key = "upload-key-under-lock"
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        original = await _upload(client, kb_id, "original.txt", b"same")
        actual_find = file_repository.find_by_sha
        calls = 0

        async def miss_once(conn: Any, looked_up_kb: Any, sha256: str) -> Any:
            nonlocal calls
            calls += 1
            if calls == 1:
                return None
            return await actual_find(conn, looked_up_kb, sha256)

        monkeypatch.setattr(file_repository, "find_by_sha", miss_once)
        duplicate = await _upload(client, kb_id, "duplicate.txt", b"same", **{"Idempotency-Key": key})
        replay = await _upload(client, kb_id, "different.txt", b"different", **{"Idempotency-Key": key})
    assert calls >= 2
    assert (original.status_code, duplicate.status_code, replay.status_code) == (201, 200, 200)
    assert duplicate.json()["data"]["id"] == replay.json()["data"]["id"] == original.json()["data"]["id"]
    assert len(list(minio.list_objects(RAW_BUCKET, recursive=True))) == 1


async def test_actual_multipart_bytes_are_bounded_when_length_is_understated(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    limits = {"upload_max_bytes": 10, "upload_multipart_overhead_bytes": 1000}
    async with api_client(pool, minio_config=minio_config, **limits) as (client, app):
        kb_id = await _kb(client, app)
        boundary = "b"
        body = (
            b"--b\r\nX-Padding: "
            + b"x" * 1200
            + b'\r\nContent-Disposition: form-data; name="file"; filename="small.txt"\r\n\r\nok\r\n--b--\r\n'
        )
        response = await client.post(
            f"/v1/knowledge-bases/{kb_id}/files",
            content=body,
            headers={
                **bearer(ORG, "kb:write"),
                "Content-Type": f"multipart/form-data; boundary={boundary}",
                "Content-Length": "100",
            },
        )
    assert response.json()["error"]["code"] == "PAYLOAD_TOO_LARGE"
    assert list(minio.list_objects(RAW_BUCKET, recursive=True)) == []


async def test_upload_limits_reject_before_storing(
    pool: Pool, minio_config: dict[str, Any], minio: Minio
) -> None:
    limits = {"upload_max_bytes": 10, "upload_multipart_overhead_bytes": 1000}
    async with api_client(pool, minio_config=minio_config, **limits) as (client, app):
        kb_id = await _kb(client, app)
        part_too_big = await _upload(client, kb_id, "big.txt", b"x" * 20)
        body_too_big = await _upload(client, kb_id, "huge.txt", b"x" * 2000)
        unsupported = await _upload(client, kb_id, "setup.exe", b"MZ")

        async def chunks():  # type: ignore[no-untyped-def]
            yield (
                b'--b\r\nContent-Disposition: form-data; name="file"; filename="a.txt"\r\n\r\nhi\r\n--b--\r\n'
            )

        no_length = await client.post(
            f"/v1/knowledge-bases/{kb_id}/files",
            content=chunks(),
            headers={**bearer(ORG, "kb:write"), "Content-Type": "multipart/form-data; boundary=b"},
        )
    assert part_too_big.json()["error"]["code"] == "PAYLOAD_TOO_LARGE"
    assert body_too_big.json()["error"]["code"] == "PAYLOAD_TOO_LARGE"
    assert unsupported.json()["error"]["code"] == "UNSUPPORTED_MEDIA_TYPE"
    assert no_length.json()["error"]["code"] == "VALIDATION_FAILED"
    assert list(minio.list_objects(RAW_BUCKET, recursive=True)) == []


async def test_delete_retry_and_preview(pool: Pool, minio_config: dict[str, Any], minio: Minio) -> None:
    write, read = bearer(ORG, "kb:write"), bearer(ORG, "kb:read")
    async with api_client(pool, minio_config=minio_config) as (client, app):
        kb_id = await _kb(client, app)
        file_id = (await _upload(client, kb_id, "r.txt", b"retry me")).json()["data"]["id"]
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
        kept = await _upload(client, kb_id, "kept.txt", b"first file")
        dropped = await _upload(client, kb_id, "dropped.txt", b"second file")
        assert (kept.status_code, dropped.status_code) == (201, 201)
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
            await _upload(client, kb_id, f"f{index}.txt", f"content {index}".encode())
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
        file_id = (await _upload(client, kb_id, "s.txt", b"secret")).json()["data"]["id"]
        await _kb(client, app, "org-beta")
        beta_read, beta_write = bearer("org-beta", "kb:read"), bearer("org-beta", "kb:write")
        codes = [
            (await client.get(f"/v1/knowledge-bases/{kb_id}/files", headers=beta_read)).json()["error"][
                "code"
            ],
            (await client.get(f"/v1/knowledge-bases/{kb_id}/files/{file_id}/raw", headers=beta_read)).json()[
                "error"
            ]["code"],
            (await client.delete(f"/v1/knowledge-bases/{kb_id}/files/{file_id}", headers=beta_write)).json()[
                "error"
            ]["code"],
        ]
    assert codes == ["KB_NOT_FOUND"] * 3
