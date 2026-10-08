import json
import threading
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from types import SimpleNamespace
from typing import ClassVar
from unittest.mock import Mock

import pytest
import structlog
from fastapi.testclient import TestClient
from opentelemetry import trace
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

from tests.support import FakeResources
from thomas_ragx import cli
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.tracing import (
    RAGX_TRACE_ID_HEADER,
    apply_trace_content_policy,
    configure_tracing,
)

PHOENIX_KEY = "phoenix-admin-secret-0123456789abcdefghij"


class _Recorder(BaseHTTPRequestHandler):
    received: ClassVar[list[tuple[str, str]]] = []

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        self.rfile.read(length)
        _Recorder.received.append((self.path, self.headers.get("Authorization", "")))
        self.send_response(200)
        self.end_headers()

    def log_message(self, format: str, *args: object) -> None:
        return


@pytest.fixture
def otlp_server() -> Iterator[str]:
    _Recorder.received = []
    server = ThreadingHTTPServer(("127.0.0.1", 0), _Recorder)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_address[1]}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def _config(**overrides: object) -> Config:
    return Config(minio_access_key="a", minio_secret_key="s", **overrides)  # type: ignore[arg-type]


def test_exporter_sends_bearer_header_and_never_logs_it(
    otlp_server: str, capsys: pytest.CaptureFixture[str]
) -> None:
    provider = configure_tracing(_config(otlp_endpoint=otlp_server, phoenix_api_key=PHOENIX_KEY))
    try:
        with provider.get_tracer(__name__).start_as_current_span("rag.test.span"):
            pass
        assert provider.force_flush()
    finally:
        provider.shutdown()
    assert _Recorder.received == [("/v1/traces", f"Bearer {PHOENIX_KEY}")]
    captured = capsys.readouterr()
    assert PHOENIX_KEY not in captured.out + captured.err


@pytest.mark.parametrize(
    "suffix", ["\nsecret-tail", "\rsecret-tail", "\x00secret-tail", "\x7fsecret-tail", "☃", " ", "\t"]
)
def test_malformed_bearer_key_is_rejected_without_leaking_secret(
    otlp_server: str, caplog: pytest.LogCaptureFixture, suffix: str
) -> None:
    malformed_key = PHOENIX_KEY + suffix
    with pytest.raises(AppError) as rejected:
        provider = configure_tracing(_config(otlp_endpoint=otlp_server, phoenix_api_key=malformed_key))
        try:
            with provider.get_tracer(__name__).start_as_current_span("rag.test.malformed-key"):
                pass
            provider.force_flush()
        finally:
            provider.shutdown()
    assert rejected.value.code == ErrorCode.VALIDATION_FAILED
    assert rejected.value.__cause__ is None
    assert PHOENIX_KEY not in str(rejected.value) + repr(rejected.value) + caplog.text
    assert not _Recorder.received


@pytest.mark.parametrize("key", [None, ""])
def test_missing_bearer_key_does_not_prevent_export(otlp_server: str, key: str | None) -> None:
    provider = configure_tracing(_config(otlp_endpoint=otlp_server, phoenix_api_key=key))
    try:
        with provider.get_tracer(__name__).start_as_current_span("rag.test.no-key"):
            pass
        assert provider.force_flush()
    finally:
        provider.shutdown()
    assert _Recorder.received == [("/v1/traces", "")]


def test_resource_names_the_phoenix_project() -> None:
    provider = configure_tracing(_config(phoenix_project="thomas-ragx-test"))
    try:
        assert provider.resource.attributes["openinference.project.name"] == "thomas-ragx-test"
    finally:
        provider.shutdown()


def test_trace_content_policy_hides_by_default() -> None:
    environ: dict[str, str] = {}
    apply_trace_content_policy(_config(), environ)
    assert environ == {"OPENINFERENCE_HIDE_INPUTS": "true", "OPENINFERENCE_HIDE_OUTPUTS": "true"}
    apply_trace_content_policy(_config(trace_content=True), environ)
    assert environ == {"OPENINFERENCE_HIDE_INPUTS": "false", "OPENINFERENCE_HIDE_OUTPUTS": "false"}


def test_create_app_does_not_install_global_provider() -> None:
    before = trace.get_tracer_provider()
    first, second = (
        create_app(_config(), resources=FakeResources()),
        create_app(_config(), resources=FakeResources()),
    )
    try:
        assert trace.get_tracer_provider() is before
        assert first.state.tracer_provider is not second.state.tracer_provider
    finally:
        first.state.tracer_provider.shutdown()
        second.state.tracer_provider.shutdown()


def test_root_span_carries_validated_request_id_and_returns_trace_id(
    capsys: pytest.CaptureFixture[str],
) -> None:
    previous = structlog.contextvars.get_contextvars()
    structlog.contextvars.clear_contextvars()
    app = create_app(_config(), resources=FakeResources())
    exporter = InMemorySpanExporter()
    app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))
    try:
        with TestClient(app) as client:
            forged = client.get("/healthz", headers={"X-Request-Id": 'req_x"} forged'})
            valid = client.get(
                "/healthz", headers={"X-Request-Id": "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"}
            )
        spans = exporter.get_finished_spans()
        assert len(spans) == 2
        for response, span in zip((forged, valid), spans, strict=True):
            trace_id = response.headers[RAGX_TRACE_ID_HEADER]
            assert len(trace_id) == 32
            assert int(trace_id, 16) != 0
            assert trace_id == trace_id.lower()
            assert span.context is not None
            assert span.context.trace_id == int(trace_id, 16)
            assert span.name == "rag.http.request"
            assert span.attributes is not None
            assert span.attributes["request_id"] == response.headers["X-Request-Id"]
            assert span.attributes["openinference.span.kind"] == "CHAIN"
            assert "org_id" not in span.attributes
        records = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
        completed = [record for record in records if record["msg"] == "request completed"]
        assert completed[0]["trace_id"] == forged.headers[RAGX_TRACE_ID_HEADER]
        assert completed[0]["request_id"] == forged.headers["X-Request-Id"]
        assert "forged" not in completed[0]["request_id"]
        assert completed[1]["request_id"] == "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
    finally:
        app.state.tracer_provider.shutdown()
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(**previous)


def test_requests_succeed_when_phoenix_is_unreachable() -> None:
    app = create_app(
        _config(otlp_endpoint="http://127.0.0.1:9", phoenix_api_key=PHOENIX_KEY), resources=FakeResources()
    )
    try:
        with TestClient(app) as client:
            assert client.get("/healthz").status_code == 200
    finally:
        app.state.tracer_provider.shutdown()


@pytest.mark.parametrize("server_fails", [False, True])
def test_serve_installs_provider_and_enforces_content_policy_before_server(
    monkeypatch: pytest.MonkeyPatch, server_fails: bool
) -> None:
    config = _config()
    provider = Mock()
    api = SimpleNamespace(state=SimpleNamespace(tracer_provider=provider))
    install = Mock()
    monkeypatch.setenv("OPENINFERENCE_HIDE_INPUTS", "false")
    monkeypatch.setenv("OPENINFERENCE_HIDE_OUTPUTS", "false")
    monkeypatch.setattr(cli, "load_config", lambda: config)

    def create(config: Config) -> SimpleNamespace:
        assert cli.os.environ["OPENINFERENCE_HIDE_INPUTS"] == "true"
        assert cli.os.environ["OPENINFERENCE_HIDE_OUTPUTS"] == "true"
        return api

    def run(app: SimpleNamespace, *, host: str, port: int, log_config: None) -> None:
        assert app is api
        assert host == config.http_host
        assert port == config.http_port
        install.assert_called_once_with(provider)
        provider.shutdown.assert_not_called()
        if server_fails:
            raise RuntimeError("server failed")

    monkeypatch.setattr(cli, "create_app", create)
    monkeypatch.setattr(cli, "install_global_tracer_provider", install)
    monkeypatch.setattr(cli.uvicorn, "run", run)
    if server_fails:
        with pytest.raises(RuntimeError, match="server failed"):
            cli.serve()
    else:
        cli.serve()
    provider.shutdown.assert_called_once_with()


@pytest.mark.parametrize("path, status", [("/missing", 404), ("/app-error", 400), ("/unhandled", 500)])
def test_error_responses_keep_trace_header_and_safe_error_body(path: str, status: int) -> None:
    app = create_app(_config(), resources=FakeResources())
    exporter = InMemorySpanExporter()
    app.state.tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))

    @app.get("/app-error")
    def app_error() -> None:
        raise AppError(ErrorCode.VALIDATION_FAILED)

    @app.get("/unhandled")
    def unhandled() -> None:
        raise ValueError(PHOENIX_KEY)

    try:
        with TestClient(app) as client:
            response = client.get(path, headers={"X-Request-Id": "req_trace-error-0001"})
        assert response.status_code == status
        assert PHOENIX_KEY not in response.text
        assert response.json()["error"]["requestId"] == "req_trace-error-0001"
        assert response.headers["X-Request-Id"] == "req_trace-error-0001"
        trace_id = response.headers[RAGX_TRACE_ID_HEADER]
        assert len(trace_id) == 32
        assert trace_id == trace_id.lower()
        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        assert spans[0].context is not None
        assert spans[0].context.trace_id == int(trace_id, 16)
        assert spans[0].attributes is not None
        assert spans[0].attributes["request_id"] == "req_trace-error-0001"
        assert not spans[0].events
        if status == 500:
            assert response.json()["error"]["code"] == "INTERNAL_ERROR"
    finally:
        app.state.tracer_provider.shutdown()
