from fastapi import FastAPI
from fastapi.testclient import TestClient

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import install_error_handlers


def build_app() -> FastAPI:
    app = FastAPI()
    install_error_handlers(app)

    @app.get("/conflict")
    def conflict() -> None:
        raise AppError(ErrorCode.CONFLICT, details={"field": "name"})

    @app.get("/boom")
    def boom() -> None:
        raise RuntimeError("database password=hunter2 exploded")

    @app.get("/items/{item_id}")
    def item(item_id: int) -> dict[str, int]:
        return {"id": item_id}

    return app


client = TestClient(build_app(), raise_server_exceptions=False)


def test_error_response_uses_catalog_status_and_language() -> None:
    res = client.get("/conflict", headers={"Accept-Language": "en"})
    assert res.status_code == 409
    body = res.json()["error"]
    assert body["code"] == "CONFLICT"
    assert body["message"] == "The request conflicts with the current state."
    assert body["details"] == {"field": "name"}


def test_error_response_hides_unexpected_error_details() -> None:
    res = client.get("/boom")
    assert res.status_code == 500
    assert res.json()["error"]["code"] == "INTERNAL_ERROR"
    assert "hunter2" not in res.text


def test_error_response_wraps_validation_errors() -> None:
    res = client.get("/items/not-a-number")
    assert res.status_code == 400
    body = res.json()["error"]
    assert body["code"] == "VALIDATION_FAILED"
    assert body["details"]["fields"][0]["field"] == "item_id"


def test_error_response_wraps_unknown_routes() -> None:
    res = client.get("/nope")
    assert res.status_code == 404
    assert res.json()["error"]["code"] == "NOT_FOUND"


def test_error_body_carries_request_id_not_trace_id() -> None:
    app = create_app(Config(minio_access_key="a", minio_secret_key="s"), resources=FakeResources())

    @app.get("/conflict")
    def conflict() -> None:
        raise AppError(ErrorCode.CONFLICT)

    sent = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
    try:
        with TestClient(app) as client:
            response = client.get("/conflict", headers={"X-Request-Id": sent})
        assert response.json()["error"]["requestId"] == sent
        assert "traceId" not in response.json()["error"]
    finally:
        app.state.tracer_provider.shutdown()


def test_app_replaces_forged_request_id() -> None:
    app = create_app(Config(minio_access_key="a", minio_secret_key="s"), resources=FakeResources())
    try:
        with TestClient(app) as client:
            response = client.get("/healthz", headers={"X-Request-Id": 'req_x"} injected'})
        assert response.headers["X-Request-Id"].startswith("req_")
        assert "injected" not in response.headers["X-Request-Id"]
    finally:
        app.state.tracer_provider.shutdown()
