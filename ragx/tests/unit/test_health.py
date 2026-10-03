from fastapi.testclient import TestClient

from thomas_ragx.api.app import create_app
from thomas_ragx.platform.settings import Settings


def test_healthz_returns_envelope_with_request_id() -> None:
    app = create_app(Settings(core_service_token="t", minio_access_key="a", minio_secret_key="s"))
    res = TestClient(app).get("/healthz")
    assert res.status_code == 200
    assert res.json()["data"] == {"status": "ok"}
    assert res.json()["meta"]["requestId"] == res.headers["X-Request-Id"]
