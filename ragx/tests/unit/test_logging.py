import json
from collections.abc import Iterator
from unittest.mock import Mock

import httpx
import pytest
import structlog
from typer.testing import CliRunner

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx import cli
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.logging import REDACTED, redact


def test_redact_hides_secret_keys_and_values() -> None:
    event = {
        "event": "calling provider",
        "api_key": "sk-or-v1-abc",
        "headers": {"Authorization": "Bearer xyz", "accept": "json"},
        "note": "token sk-live-123 leaked",
        "input_tokens": 392,
    }
    out = redact(None, "info", event)
    assert out["input_tokens"] == 392
    assert out["api_key"] == REDACTED
    assert out["headers"]["Authorization"] == REDACTED
    assert out["headers"]["accept"] == "json"
    assert "sk-live-123" not in out["note"]


def test_redact_hides_secrets_in_nested_containers() -> None:
    details = {
        "providers": [{"api_key": "plain-secret", "input_tokens": 392}],
        "nested": ([{"password": "plain-secret"}],),
        "values": ["Bearer plain-secret"],
        "token": [{"value": "plain-secret"}],
    }
    cleaned = redact(None, "info", {"details": details})["details"]
    assert "plain-secret" not in str(cleaned)
    assert cleaned["providers"][0]["api_key"] == REDACTED
    assert cleaned["providers"][0]["input_tokens"] == 392
    assert cleaned["token"] == REDACTED
    assert isinstance(cleaned["nested"], tuple)
    assert details["providers"][0]["api_key"] == "plain-secret"


@pytest.fixture
def isolated_logging() -> Iterator[None]:
    configuration = structlog.get_config()
    context = structlog.contextvars.get_contextvars()
    structlog.contextvars.clear_contextvars()
    try:
        yield
    finally:
        structlog.configure(**configuration)
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(**context)


def test_cli_bootstrap_logs_environment_and_active_trace(
    monkeypatch: pytest.MonkeyPatch, isolated_logging: None
) -> None:
    config = Config(
        environment="logging-test",
        core_service_token="t",
        minio_access_key="a",
        minio_secret_key="s",
        otlp_endpoint="",
    )
    storage = Mock()
    storage.bucket_exists.return_value = False
    monkeypatch.setattr(cli, "load_config", lambda: config)
    monkeypatch.setattr(bootstrap_module, "log", structlog.get_logger(bootstrap_module.__name__))
    monkeypatch.setattr(bootstrap_module, "Minio", Mock(return_value=storage))
    monkeypatch.setattr(
        bootstrap_module.httpx,
        "get",
        Mock(return_value=httpx.Response(200, request=httpx.Request("GET", config.qdrant_url))),
    )

    result = CliRunner().invoke(cli.app, ["bootstrap"])

    assert result.exit_code == 0, result.output
    records = [json.loads(line) for line in result.stdout.splitlines()]
    bucket_records = [record for record in records if record["msg"] == "bucket created"]
    assert {record["bucket"] for record in bucket_records} == set(config.buckets)
    assert any(record["msg"] == "bootstrap complete" for record in records)
    for record in records:
        assert record["env"] == config.environment
        assert record["service"] == config.service_name
        assert record["level"] == "info"
        assert record["ts"]
        assert record["msg"]
        assert len(record["trace_id"]) == 32
        assert len(record["span_id"]) == 16
        assert int(record["trace_id"], 16) != 0
        assert int(record["span_id"], 16) != 0
