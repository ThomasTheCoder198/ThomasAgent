import asyncio
import hashlib
import threading
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock
from uuid import uuid4

import pytest
from opentelemetry.sdk.trace import TracerProvider

from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.ingest.source import fetch_verified
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


async def test_cancelled_fetch_removes_file_after_download_thread_finishes(tmp_path: Path) -> None:
    started = threading.Event()
    release = threading.Event()
    finished = threading.Event()
    content = b"raw content"
    response = Mock()

    def read(_size: int) -> bytes:
        started.set()
        assert release.wait(timeout=5)
        finished.set()
        return content if response.read.call_count == 1 else b""

    response.read.side_effect = read
    client = Mock()
    client.get_object.return_value = response
    target = SimpleNamespace(
        file_id=uuid4(),
        kb_id=uuid4(),
        org_id="test-org",
        version=1,
        attempt_id=uuid4(),
        name="source.md",
        sha256=hashlib.sha256(content).hexdigest(),
    )
    path = tmp_path / f"{target.file_id}-{target.version}-{target.attempt_id}.md"
    task = asyncio.create_task(fetch_verified(ObjectStorage(client), target, path))
    try:
        assert await asyncio.to_thread(started.wait, 5)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
    finally:
        release.set()
    assert await asyncio.to_thread(finished.wait, 5)
    async with asyncio.timeout(5):
        while path.exists():
            await asyncio.sleep(0)
    response.close.assert_called_once()
    response.release_conn.assert_called_once()


async def test_successful_fetch_keeps_file_for_parsing(tmp_path: Path) -> None:
    content = b"raw content"
    response = Mock()
    response.read.side_effect = [content, b""]
    client = Mock()
    client.get_object.return_value = response
    target = SimpleNamespace(
        file_id=uuid4(),
        kb_id=uuid4(),
        org_id="test-org",
        version=1,
        attempt_id=uuid4(),
        name="source.md",
        sha256=hashlib.sha256(content).hexdigest(),
    )
    path = await fetch_verified(
        ObjectStorage(client), target, tmp_path / f"{target.file_id}-{target.version}-{target.attempt_id}.md"
    )
    assert path.read_bytes() == content


async def test_pipeline_cancellation_during_fetch_removes_known_path(tmp_path: Path) -> None:
    started = asyncio.Event()
    storage = Mock()

    async def download(_bucket: str, _key: str, path: Path) -> str:
        path.write_bytes(b"partial")
        started.set()
        await asyncio.Event().wait()
        return "unreachable"

    storage.download_to = download
    pipeline = IngestPipeline(
        Mock(),
        storage,
        Mock(),
        TracerProvider().get_tracer(__name__),
        indexing=Mock(),
        work_dir=tmp_path,
        lease_seconds=60,
        job_timeout_seconds=50,
    )
    target = SimpleNamespace(
        file_id=uuid4(),
        kb_id=uuid4(),
        org_id="test-org",
        version=1,
        attempt_id=uuid4(),
        name="source.md",
        sha256="unreachable",
    )
    path = tmp_path / f"{target.file_id}-{target.version}-{target.attempt_id}.md"
    task = asyncio.create_task(pipeline._run(target, "request-id"))
    await asyncio.wait_for(started.wait(), 5)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task
    assert not path.exists()


async def test_failed_ingest_step_observes_duration(tmp_path: Path) -> None:
    metrics = Mock()
    pipeline = IngestPipeline(
        Mock(),
        Mock(),
        Mock(),
        TracerProvider().get_tracer(__name__),
        indexing=Mock(),
        work_dir=tmp_path,
        lease_seconds=60,
        job_timeout_seconds=50,
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
