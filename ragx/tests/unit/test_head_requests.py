from fastapi.testclient import TestClient

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config

KB_FILES = "/v1/knowledge-bases/3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21/files"


def _client() -> TestClient:
    app = create_app(
        Config(minio_access_key="a", minio_secret_key="s", otlp_endpoint=""), resources=FakeResources()
    )
    return TestClient(app)


def test_head_on_a_get_route_answers_like_get_without_a_body() -> None:
    client = _client()
    for path in ("/healthz", "/readyz"):
        got, head = client.get(path), client.head(path)
        assert head.status_code == got.status_code == 200, path
        assert head.content == b"", path
        assert head.headers["content-type"] == got.headers["content-type"], path
        assert head.headers["content-length"] == got.headers["content-length"], path


def test_head_hits_the_same_auth_check_as_get() -> None:
    client = _client()
    got, head = client.get(KB_FILES), client.head(KB_FILES)
    assert got.json()["error"]["code"] == "APP_TOKEN_INVALID"
    assert head.status_code == got.status_code == 401
    assert head.content == b""


def test_a_method_a_route_does_not_serve_is_405_with_the_shared_error_body() -> None:
    client = _client()
    for response in (client.put("/healthz"), client.delete("/v1/manifest")):
        assert response.status_code == 405
        error = response.json()["error"]
        assert error["code"] == "METHOD_NOT_ALLOWED"
        assert error["requestId"]
