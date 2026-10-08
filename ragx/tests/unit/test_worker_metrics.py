import pytest

from thomas_ragx.platform.config import Config
from thomas_ragx.worker.metrics import (
    JOB_COUNT_NAME,
    JOB_DURATION_NAME,
    OUTCOME_RETRY,
    RELAY_BATCH_COUNT_NAME,
    RELAY_DURATION_NAME,
    RELAY_ROWS_NAME,
    configure_worker_metrics,
)


def test_worker_metrics_export_to_stdout_with_bounded_labels(capfd: pytest.CaptureFixture[str]) -> None:
    config = Config(minio_access_key="a", minio_secret_key="s")
    provider, metrics = configure_worker_metrics(config)
    metrics.observe_job("thomas.ingest.requested", OUTCOME_RETRY, 0.25)
    metrics.observe_relay("success", 0.1, 2)
    provider.force_flush()
    output = capfd.readouterr().out
    provider.shutdown()

    assert JOB_COUNT_NAME in output
    assert JOB_DURATION_NAME in output
    assert RELAY_BATCH_COUNT_NAME in output
    assert RELAY_DURATION_NAME in output
    assert RELAY_ROWS_NAME in output
    assert "thomas.ingest.requested" in output
    assert OUTCOME_RETRY in output
    assert "request_id" not in output
