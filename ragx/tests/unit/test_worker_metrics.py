import json
from typing import Any

import pytest

from thomas_ragx.platform.config import Config
from thomas_ragx.worker.metrics import (
    HTTP_DURATION_NAME,
    JOB_COUNT_NAME,
    JOB_DURATION_BUCKETS_SECONDS,
    JOB_DURATION_NAME,
    OUTCOME_RETRY,
    RELAY_BATCH_COUNT_NAME,
    RELAY_DURATION_NAME,
    RELAY_ROWS_NAME,
    SWEEP_COUNT_NAME,
    SWEEP_KIND_SLOT,
    configure_worker_metrics,
)


def _metrics_export(line: str) -> dict[str, Any] | None:
    """Stdout also carries structlog JSON lines and may carry stray text: keep only metric exports."""
    try:
        parsed = json.loads(line)
    except ValueError:
        return None
    return parsed if isinstance(parsed, dict) and "resource_metrics" in parsed else None


def test_worker_metrics_export_to_stdout_with_bounded_labels(capfd: pytest.CaptureFixture[str]) -> None:
    config = Config(minio_access_key="a", minio_secret_key="s")
    provider, metrics = configure_worker_metrics(config)
    metrics.observe_job("thomas.ingest.requested", OUTCOME_RETRY, 0.25)
    metrics.observe_relay("success", 0.1, 2)
    metrics.observe_sweep(SWEEP_KIND_SLOT, 3)
    provider.force_flush()
    output = capfd.readouterr().out
    provider.shutdown()

    assert JOB_COUNT_NAME in output
    assert JOB_DURATION_NAME in output
    assert RELAY_BATCH_COUNT_NAME in output
    assert RELAY_DURATION_NAME in output
    assert RELAY_ROWS_NAME in output
    assert SWEEP_COUNT_NAME in output
    assert "thomas.ingest.requested" in output
    assert OUTCOME_RETRY in output
    assert "request_id" not in output


def test_http_latency_histogram_uses_second_scale_buckets(capfd: pytest.CaptureFixture[str]) -> None:
    config = Config(minio_access_key="a", minio_secret_key="s")
    provider, _ = configure_worker_metrics(config)
    provider.get_meter("http").create_histogram(HTTP_DURATION_NAME, unit="s").record(0.2)
    provider.force_flush()
    output = capfd.readouterr().out
    provider.shutdown()

    exports = [export for line in output.splitlines() if (export := _metrics_export(line)) is not None]
    bounds = [
        point["explicit_bounds"]
        for export in exports
        for scope in export["resource_metrics"][0]["scope_metrics"]
        for metric in scope["metrics"]
        if metric["name"] == HTTP_DURATION_NAME
        for point in metric["data"]["data_points"]
    ]
    assert bounds == [list(JOB_DURATION_BUCKETS_SECONDS)]
