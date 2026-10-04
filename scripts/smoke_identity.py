"""Check the deployed identity/registry flow without retaining credentials."""

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
REQUEST_TIMEOUT_SECONDS = 20
SUCCESS = 200
CREATED = 201
FORBIDDEN = 403


class SmokeFailure(Exception):
    """Report a probe name without exposing response bodies or credentials."""


def request(opener, method, path, payload=None, csrf=None):
    headers = {"Content-Type": "application/json"}
    if csrf:
        headers["X-CSRF-Token"] = csrf
    body = None if payload is None else json.dumps(payload).encode()
    req = Request(CORE_URL + path, data=body, headers=headers, method=method)
    try:
        response = opener.open(req, timeout=REQUEST_TIMEOUT_SECONDS)
    except HTTPError as error:
        response = error
    with response:
        return response.status, json.loads(response.read())


def expect(name, status, expected):
    if status != expected:
        raise SmokeFailure(f"{name} (HTTP {status})")
    print(f"ok   {name}")


def probe_identity():
    env_path = Path(__file__).resolve().parents[1] / "deploy/compose/.env"
    settings = {**dotenv_values(env_path), **os.environ}
    email = settings.get("CORE_AUTH_OWNER_EMAIL")
    password = settings.get("CORE_AUTH_OWNER_PASSWORD")
    if not email or not password:
        raise SmokeFailure("missing owner bootstrap credentials")
    opener = build_opener(HTTPCookieProcessor(CookieJar()))
    status, login = request(opener, "POST", "/api/v1/auth/login", {"email": email, "password": password})
    expect("login", status, SUCCESS)
    csrf = login["data"]["csrfToken"]
    status, me = request(opener, "GET", "/api/v1/auth/me")
    if me.get("data", {}).get("user", {}).get("email") != email.strip().lower():
        raise SmokeFailure("me identity mismatch")
    expect("me", status, SUCCESS)
    payload = {"kind": "openrouter", "name": "smoke-" + uuid.uuid4().hex}
    status, denied = request(opener, "POST", "/api/v1/providers", payload)
    if denied.get("error", {}).get("code") != "AUTH_CSRF_INVALID":
        raise SmokeFailure("csrf-required code mismatch")
    expect("csrf-required", status, FORBIDDEN)
    status, created = request(opener, "POST", "/api/v1/providers", payload, csrf)
    if status != CREATED:
        raise SmokeFailure(f"provider-create (HTTP {status})")
    provider_id = created["data"]["id"]
    try:
        if created["data"].get("hasApiKey") is not False or "apiKey" in created["data"]:
            raise SmokeFailure("provider-create key visibility")
        expect("provider-create", status, CREATED)
    finally:
        status, _ = request(opener, "DELETE", f"/api/v1/providers/{provider_id}", csrf=csrf)
        expect("provider-cleanup", status, SUCCESS)
    status, _ = request(opener, "POST", "/api/v1/auth/logout", csrf=csrf)
    expect("logout", status, SUCCESS)


if __name__ == "__main__":
    try:
        probe_identity()
    except (SmokeFailure, URLError, ValueError, KeyError, OSError) as error:
        detail = str(error) if isinstance(error, SmokeFailure) else type(error).__name__
        print(f"FAIL {detail}", file=sys.stderr)
        sys.exit(1)
