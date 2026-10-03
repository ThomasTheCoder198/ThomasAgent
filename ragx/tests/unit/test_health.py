import json

import pytest
import structlog
from fastapi import Request
from fastapi.testclient import TestClient

from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.http_response import request_id_of, success_body


def test_healthz_returns_response_with_request_id() -> None:
    app = create_app(Config(core_service_token="t", minio_access_key="a", minio_secret_key="s"))
    res = TestClient(app).get("/healthz")
    assert res.status_code == 200
    assert res.json()["data"] == {"status": "ok"}
    assert res.json()["meta"]["requestId"] == res.headers["X-Request-Id"]


@pytest.mark.parametrize("incoming_request_id", [None, "req-client"])
def test_app_logs_environment_and_correlates_handler_with_response(
    capsys: pytest.CaptureFixture[str], incoming_request_id: str | None
) -> None:
    previous_context = structlog.contextvars.get_contextvars()
    structlog.contextvars.clear_contextvars()
    config = Config(
        environment="logging-test",
        core_service_token="t",
        minio_access_key="a",
        minio_secret_key="s",
        otlp_endpoint="",
    )
    app = create_app(config)

    @app.get("/logged")
    def logged(request: Request) -> dict[str, object]:
        structlog.get_logger(__name__).info("handler completed")
        return success_body({"status": "ok"}, request_id_of(request))

    try:
        headers = {"X-Request-Id": incoming_request_id} if incoming_request_id else {}
        with TestClient(app) as client:
            response = client.get("/logged", headers=headers)
        assert response.status_code == 200
        response_request_id = response.headers["X-Request-Id"]
        assert response.json() == {"data": {"status": "ok"}, "meta": {"requestId": response_request_id}}
        if incoming_request_id:
            assert response_request_id == incoming_request_id
        records = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
        assert {record["msg"] for record in records} == {"handler completed", "request completed"}
        for record in records:
            assert record["env"] == config.environment
            assert record["service"] == config.service_name
            assert record["level"] == "info"
            assert record["ts"]
            assert record["request_id"] == response_request_id
            assert len(record["trace_id"]) == 32
            assert len(record["span_id"]) == 16
            assert int(record["trace_id"], 16) != 0
            assert int(record["span_id"], 16) != 0
        assert records[0]["trace_id"] == records[1]["trace_id"]
        assert records[0]["span_id"] == records[1]["span_id"]
    finally:
        app.state.tracer_provider.shutdown()
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(**previous_context)
