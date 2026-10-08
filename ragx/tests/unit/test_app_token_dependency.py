import json

import jwt
import pytest
from fastapi import Request
from fastapi.testclient import TestClient

from tests.support import FakeResources
from tests.tokens import mint, public_keys_config
from thomas_ragx.api.app import create_app
from thomas_ragx.auth.caller import Caller
from thomas_ragx.auth.dependencies import KbReadCaller
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.http_response import request_id_of, success_body


def _client() -> TestClient:
    config = Config(
        minio_access_key="a", minio_secret_key="s", otlp_endpoint="", core_public_keys=public_keys_config()
    )
    app = create_app(config, resources=FakeResources())

    @app.get("/v1/probe")
    async def probe(caller: KbReadCaller, request: Request) -> dict[str, object]:
        assert isinstance(caller, Caller)
        return success_body({"orgId": caller.org_id}, request_id_of(request))

    return TestClient(app)


def _records(capsys: pytest.CaptureFixture[str]) -> list[dict[str, object]]:
    return [json.loads(line) for line in capsys.readouterr().out.splitlines() if line.startswith("{")]


def _headers(token: str, request_id: str | None = None) -> dict[str, str]:
    claims = jwt.decode(token, options={"verify_signature": False})
    return {
        "Authorization": f"Bearer {token}",
        "X-Request-Id": request_id or claims["rid"],
    }


def test_signed_request_id_must_match_header(capsys: pytest.CaptureFixture[str]) -> None:
    token = mint("org-alpha", "kb:read")
    response = _client().get("/v1/probe", headers=_headers(token, "req_aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "APP_TOKEN_INVALID"
    rejected = next(r for r in _records(capsys) if r["msg"] == "app token rejected")
    assert rejected["reason"] == "invalid_claim"
    assert token not in json.dumps(rejected)


def test_signed_request_id_requires_header(capsys: pytest.CaptureFixture[str]) -> None:
    token = mint("org-alpha", "kb:read")
    response = _client().get("/v1/probe", headers={"Authorization": f"Bearer {token}"})
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "APP_TOKEN_INVALID"
    rejected = next(r for r in _records(capsys) if r["msg"] == "app token rejected")
    assert rejected["reason"] == "invalid_claim"


def test_valid_token_with_matching_scope_reaches_the_handler(capsys: pytest.CaptureFixture[str]) -> None:
    token = mint("org-alpha", "kb:read")
    response = _client().get("/v1/probe", headers=_headers(token))
    assert response.status_code == 200
    assert response.json()["data"] == {"orgId": "org-alpha"}
    completed = next(r for r in _records(capsys) if r["msg"] == "request completed")
    assert completed["org_id"] == "org-alpha"
    assert isinstance(completed["jti"], str)


def test_wrong_scope_is_forbidden() -> None:
    token = mint("org-alpha", "kb:write")
    response = _client().get("/v1/probe", headers=_headers(token))
    assert response.status_code == 403
    assert response.json()["error"]["code"] == "FORBIDDEN"


@pytest.mark.parametrize("header", [None, "Basic abc", "Bearer not-a-token"])
def test_missing_or_bad_token_is_401_and_the_token_is_never_logged(
    capsys: pytest.CaptureFixture[str], header: str | None
) -> None:
    headers = {"Authorization": header} if header else {}
    response = _client().get("/v1/probe", headers=headers)
    assert response.status_code == 401
    body = response.json()["error"]
    assert body["code"] == "APP_TOKEN_INVALID"
    assert "details" not in body
    records = _records(capsys)
    assert any(record["msg"] == "app token rejected" for record in records)
    assert "not-a-token" not in json.dumps(records)


def test_rejection_reason_is_logged_as_a_code(capsys: pytest.CaptureFixture[str]) -> None:
    expired = mint("org-alpha", "kb:read", iat=1, exp=2)
    _client().get("/v1/probe", headers={"Authorization": f"Bearer {expired}"})
    rejected = next(r for r in _records(capsys) if r["msg"] == "app token rejected")
    assert rejected["reason"] == "expired"
    assert expired not in json.dumps(rejected)
