"""End-to-end check of option C through core: org → ragx provisioning → app proxy → presigned MinIO upload → confirm
→ presigned download → cleanup. Stdlib + python-dotenv, like smoke_identity.py; never prints bodies or credentials."""

import json
import os
import sys
import uuid
from http.cookiejar import CookieJar
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import HTTPCookieProcessor, Request, build_opener

from dotenv import dotenv_values

CORE_URL = "http://localhost:8080"
FRONTEND_ORIGIN = os.environ.get("SMOKE_WEB_ORIGIN", "http://localhost:3000")
REQUEST_TIMEOUT_SECONDS = 40  # one synchronous provisioning call fits CORE_HTTP_REQUEST_TIMEOUT (30s)
OK, CREATED, NO_CONTENT = 200, 201, 204
SMOKE_ORG_NAME = "Smoke"
SMOKE_FILE_NAME = "Tài liệu smoke.md"
SMOKE_FILE_BODY = f"# Smoke\n\nThomasAgent option C check {uuid.uuid4()}\n".encode()  # unique: never a duplicate
ATTACHMENT = "attachment"
OCTET_STREAM = "application/octet-stream"
CORS_ALLOW_ORIGIN = "Access-Control-Allow-Origin"
ANY_ORIGIN = "*"


class SmokeFailure(Exception):
    """A probe name, never a response body."""


def call(opener, method, url, payload=None, csrf=None, body=None, headers=None):
    sent = dict(headers or {})
    data = body
    if payload is not None:
        data = json.dumps(payload).encode()
        sent["Content-Type"] = "application/json"
    if csrf:
        sent["X-CSRF-Token"] = csrf
    try:
        response = opener.open(Request(url, data=data, headers=sent, method=method), timeout=REQUEST_TIMEOUT_SECONDS)
    except HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


def expect(name, status, wanted):
    if status != wanted:
        raise SmokeFailure(f"{name} (HTTP {status})")
    print(f"ok   {name}")


def data_of(raw):
    return json.loads(raw)["data"]


def login(core):
    env = {**dotenv_values(Path(__file__).resolve().parents[1] / "deploy/core/.env"), **os.environ}
    credentials = {"email": env.get("CORE_AUTH_OWNER_EMAIL"), "password": env.get("CORE_AUTH_OWNER_PASSWORD")}
    status, _, raw = call(core, "POST", f"{CORE_URL}/api/v1/auth/login", credentials)
    expect("ragx-app-login", status, OK)
    return data_of(raw)["csrfToken"]


def ready_org(core, csrf):
    """Reuse one org: orgs cannot be deleted yet, so a new one per run would pile up in core and ragx."""
    status, _, raw = call(core, "GET", f"{CORE_URL}/api/v1/orgs")
    expect("ragx-app-org-list", status, OK)
    org = next((o for o in data_of(raw) if o["name"] == SMOKE_ORG_NAME), None)
    if org is None:
        status, _, raw = call(core, "POST", f"{CORE_URL}/api/v1/orgs", {"name": SMOKE_ORG_NAME, "externalParsing": False}, csrf)
        expect("ragx-app-org-create", status, CREATED)
        org = data_of(raw)
    if org["ragxStatus"] != "ready" or not org["settingsSynced"]:
        status, _, raw = call(core, "POST", f"{CORE_URL}/api/v1/orgs/{org['id']}/provision", csrf=csrf)
        expect("ragx-app-org-provision", status, OK)
        org = data_of(raw)
    if org["ragxStatus"] != "ready":
        raise SmokeFailure(f"ragx-app-org-ready ({org['ragxStatus']}: {org['ragxErrorCode']})")
    print("ok   ragx-app-org-ready")
    return org["id"]


def multipart(fields, file_name, content):
    """Policy fields first, the file last: S3 POST ignores fields after the file."""
    boundary = uuid.uuid4().hex
    parts = [
        f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode()
        for name, value in fields.items()
    ]
    head = f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{file_name}"\r\n\r\n'.encode()
    return f"multipart/form-data; boundary={boundary}", b"".join(parts) + head + content + f"\r\n--{boundary}--\r\n".encode()


def probe():
    core = build_opener(HTTPCookieProcessor(CookieJar()))
    storage = build_opener()  # no cookie jar: the browser → MinIO leg carries no credentials
    csrf = login(core)
    app = f"{CORE_URL}/api/v1/orgs/{ready_org(core, csrf)}/apps/ragx"
    status, _, raw = call(core, "GET", f"{app}/knowledge-bases")
    expect("ragx-app-list-kbs", status, OK)
    kb_id = next(kb["id"] for kb in data_of(raw) if kb["isDefault"])
    files = f"{app}/knowledge-bases/{kb_id}/files"
    status, _, raw = call(core, "POST", f"{app}/knowledge-bases/{kb_id}/uploads",
                          {"name": SMOKE_FILE_NAME, "sizeBytes": len(SMOKE_FILE_BODY)}, csrf)
    expect("ragx-app-presign", status, CREATED)
    ticket = data_of(raw)
    status, headers, _ = call(storage, "OPTIONS", ticket["upload"]["url"],
                              headers={"Origin": FRONTEND_ORIGIN, "Access-Control-Request-Method": "POST"})
    if headers.get(CORS_ALLOW_ORIGIN) not in (FRONTEND_ORIGIN, ANY_ORIGIN):
        raise SmokeFailure("minio-cors-preflight (set MINIO_API_CORS_ALLOW_ORIGIN to the frontend origin)")
    print("ok   minio-cors-preflight")
    content_type, body = multipart(ticket["upload"]["fields"], SMOKE_FILE_NAME, SMOKE_FILE_BODY)
    status, _, _ = call(storage, "POST", ticket["upload"]["url"], body=body,
                        headers={"Content-Type": content_type, "Origin": FRONTEND_ORIGIN})
    expect("minio-post", status, NO_CONTENT)
    file_id = ticket["fileId"]
    # Confirm comes first and outside the cleanup: a file that was never confirmed is still `uploading` and invisible
    # to delete, so a DELETE after a failed confirm would only answer 404 and hide the real failure.
    status, _, raw = call(core, "POST", f"{files}/{file_id}/confirm", csrf=csrf)
    if status != OK or data_of(raw)["status"] != "pending":
        raise SmokeFailure(f"ragx-app-confirm-pending (HTTP {status})")
    print("ok   ragx-app-confirm-pending")
    delete_url = f"{files}/{file_id}"
    try:
        status, _, raw = call(core, "GET", f"{delete_url}/download-url")
        expect("ragx-app-download-url", status, OK)
        status, headers, content = call(storage, "GET", data_of(raw)["url"])
        if (status != OK or content != SMOKE_FILE_BODY or headers.get("Content-Type") != OCTET_STREAM
                or not headers.get("Content-Disposition", "").startswith(ATTACHMENT)):
            raise SmokeFailure("ragx-app-download")
        print("ok   ragx-app-download")
    except BaseException:
        call(core, "DELETE", delete_url, csrf=csrf)  # best effort: the first failure is the one to report
        raise
    status, _, _ = call(core, "DELETE", delete_url, csrf=csrf)
    expect("ragx-app-cleanup", status, NO_CONTENT)


if __name__ == "__main__":
    try:
        probe()
    except (SmokeFailure, URLError, ValueError, KeyError, StopIteration, OSError) as error:
        print(f"FAIL {error if isinstance(error, SmokeFailure) else type(error).__name__}", file=sys.stderr)
        sys.exit(1)
