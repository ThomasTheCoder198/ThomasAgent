import json
from contextlib import asynccontextmanager
from pathlib import Path
from types import SimpleNamespace
from typing import Any

import pytest
from typer.testing import CliRunner

from thomas_ragx import cli

GOLDEN_LINE = {"id": "x-1", "lang": "en", "category": "A2", "question": "q?", "expected": []}


class _Provider:
    def __init__(self) -> None:
        self.shut_down = False

    def get_tracer(self, _: str) -> object:
        return object()

    def shutdown(self) -> None:
        self.shut_down = True


class _Pool:
    @asynccontextmanager
    async def connection(self) -> Any:
        yield object()


class _Resources:
    def __init__(self, *, open_error: Exception | None = None) -> None:
        self.pool = _Pool()
        self._open_error = open_error
        self.closed = False

    async def open(self) -> None:
        if self._open_error:
            raise self._open_error

    async def close(self) -> None:
        self.closed = True


class _Wiring:
    def __init__(self, monkeypatch: pytest.MonkeyPatch, resources: _Resources, kb: object | None) -> None:
        self.tracing = _Provider()
        self.metrics = _Provider()
        monkeypatch.setenv("RAG_MINIO_ACCESS_KEY", "a")
        monkeypatch.setenv("RAG_MINIO_SECRET_KEY", "s")
        cli.load_config.cache_clear()
        monkeypatch.setattr(cli.Resources, "from_config", lambda _config: resources)
        monkeypatch.setattr(cli, "configure_tracing", lambda _config: self.tracing)
        monkeypatch.setattr(cli, "install_global_tracer_provider", lambda _provider: None)
        monkeypatch.setattr(cli, "instrument_litellm", lambda _provider: None)
        monkeypatch.setattr(cli, "configure_worker_metrics", lambda _config: (self.metrics, object()))
        monkeypatch.setattr(cli, "create_registry_service", lambda *_: object())
        monkeypatch.setattr(cli, "create_search_service", lambda *_: object())

        async def get_kb(*_: Any) -> object | None:
            return kb

        monkeypatch.setattr(cli.kbs, "get_kb", get_kb)


def _invoke(tmp_path: Path, *extra: str) -> Any:
    golden = tmp_path / "set.jsonl"
    golden.write_text(json.dumps(GOLDEN_LINE), encoding="utf-8")
    try:
        return CliRunner().invoke(
            cli.app,
            [
                "eval",
                "run",
                "--set",
                str(golden),
                "--org",
                "org-1",
                "--kb",
                "7a9d6c36-0000-4000-8000-000000000001",
                *extra,
            ],
        )
    finally:
        cli.load_config.cache_clear()


def _log_lines(output: str) -> list[dict[str, Any]]:
    return [json.loads(line) for line in output.splitlines() if line.startswith("{")]


def test_eval_run_reports_an_unknown_kb_without_a_traceback(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    resources = _Resources()
    wiring = _Wiring(monkeypatch, resources, kb=None)
    result = _invoke(tmp_path)
    assert result.exit_code == 1
    assert "Traceback" not in result.output
    failure = next(line for line in _log_lines(result.output) if line.get("error_code") == "KB_NOT_FOUND")
    assert failure["request_id"].startswith("job_") and "duration_ms" in failure
    assert resources.closed and wiring.tracing.shut_down and wiring.metrics.shut_down


def test_eval_run_shuts_providers_down_when_resources_fail_to_open(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    resources = _Resources(open_error=RuntimeError("postgres down"))
    wiring = _Wiring(monkeypatch, resources, kb=object())
    result = _invoke(tmp_path)
    assert result.exit_code != 0
    assert wiring.tracing.shut_down and wiring.metrics.shut_down


def test_eval_run_logs_completion_with_request_id_and_duration(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    _Wiring(monkeypatch, _Resources(), kb=object())

    class FakeRunner:
        def __init__(self, *_: Any) -> None: ...

        async def run(self, *_: Any) -> Any:
            return SimpleNamespace(to_json=lambda: {"scored": 1, "recall": {"5": 1.0}, "mrr": 1.0})

    monkeypatch.setattr(cli, "EvalRunner", FakeRunner)
    out = tmp_path / "report.json"
    result = _invoke(tmp_path, "--out", str(out))
    assert result.exit_code == 0, result.output
    finished = next(line for line in _log_lines(result.output) if line.get("msg") == "eval finished")
    assert finished["request_id"].startswith("job_") and finished["duration_ms"] >= 0
    assert json.loads(out.read_text(encoding="utf-8"))["scored"] == 1
