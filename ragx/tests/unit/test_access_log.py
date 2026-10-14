import json

import pytest
import structlog
from fastapi import Request
from fastapi.testclient import TestClient
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import request_id_of, success_body


def _client_and_exporter() -> tuple[TestClient, InMemorySpanExporter]:
    app = create_app(
        Config(minio_access_key="a", minio_secret_key="s", otlp_endpoint=""), resources=FakeResources()
    )
    exporter = InMemorySpanExporter()
    app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))

    @app.get("/items/{item_id}")
    def item(item_id: str, request: Request) -> dict[str, object]:
        if item_id == "missing":
            raise AppError(ErrorCode.KB_NOT_FOUND)
        if item_id == "broken":
            raise RuntimeError("boom")
        request.state.org_id = "org-alpha"
        request.state.jti = "jti-0001"
        request.state.tool = "kb_search"
        return success_body({"id": item_id}, request_id_of(request))

    return TestClient(app, raise_server_exceptions=False), exporter


def _completed(capsys: pytest.CaptureFixture[str]) -> dict[str, object]:
    records = [json.loads(line) for line in capsys.readouterr().out.splitlines() if line.startswith("{")]
    return next(record for record in records if record["msg"] == "request completed")


@pytest.mark.parametrize(
    ("path", "status", "level", "route"),
    [
        ("/items/one", 200, "info", "/items/{item_id}"),
        ("/items/missing", 404, "warning", "/items/{item_id}"),
        ("/nowhere", 404, "warning", "unmatched"),
        ("/items/broken", 500, "error", "/items/{item_id}"),
    ],
)
def test_access_log_level_route_and_integer_duration(
    capsys: pytest.CaptureFixture[str], path: str, status: int, level: str, route: str
) -> None:
    structlog.contextvars.clear_contextvars()
    client, _ = _client_and_exporter()
    assert client.get(path).status_code == status
    record = _completed(capsys)
    assert record["level"] == level
    assert record["route"] == route
    assert record["status"] == status
    assert isinstance(record["duration_ms"], int)


def test_access_log_and_root_span_carry_org_and_jti_set_by_the_handler(
    capsys: pytest.CaptureFixture[str],
) -> None:
    structlog.contextvars.clear_contextvars()
    client, exporter = _client_and_exporter()
    assert client.get("/items/one").status_code == 200
    record = _completed(capsys)
    assert record["org_id"] == "org-alpha"
    assert record["jti"] == "jti-0001"
    assert record["tool"] == "kb_search"
    root = next(span for span in exporter.get_finished_spans() if span.name == "rag.http.request")
    assert root.attributes is not None
    assert root.attributes["org_id"] == "org-alpha"
    assert root.attributes["tool"] == "kb_search"


def test_requests_without_app_token_log_no_org(capsys: pytest.CaptureFixture[str]) -> None:
    structlog.contextvars.clear_contextvars()
    client, _ = _client_and_exporter()
    client.get("/healthz")
    record = _completed(capsys)
    assert "org_id" not in record
    assert "jti" not in record
