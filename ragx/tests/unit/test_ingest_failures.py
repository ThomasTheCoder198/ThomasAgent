from pathlib import Path
from unittest.mock import Mock
from uuid import uuid4

import pytest
from opentelemetry.sdk.trace import TracerProvider

from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.kb.storage import ObjectStorage


async def test_read_failure_removes_partial_download_and_preserves_error(tmp_path: Path) -> None:
    failure = OSError("read failed")
    response = Mock()
    response.read.side_effect = [b"partial raw content", failure]
    client = Mock()
    client.get_object.return_value = response
    path = tmp_path / "source.md"
    with pytest.raises(OSError) as caught:
        await ObjectStorage(client).download_to("raw", "file", path)
    assert caught.value is failure
    assert not path.exists()
    response.close.assert_called_once()
    response.release_conn.assert_called_once()


async def test_failed_ingest_step_observes_duration(tmp_path: Path) -> None:
    metrics = Mock()
    pipeline = IngestPipeline(
        Mock(),
        Mock(),
        Mock(),
        TracerProvider().get_tracer(__name__),
        work_dir=tmp_path,
        lease_seconds=60,
        metrics=metrics,
    )
    target = Mock(kb_id=uuid4(), file_id=uuid4(), version=1)
    failure = TimeoutError("provider timeout")

    async def work() -> None:
        raise failure

    with pytest.raises(TimeoutError) as caught:
        await pipeline._step("parse", work(), target)
    assert caught.value is failure
    metrics.observe_ingest_step.assert_called_once()
    step, duration = metrics.observe_ingest_step.call_args.args
    assert step == "parse" and duration >= 0
