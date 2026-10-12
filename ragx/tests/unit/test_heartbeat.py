import asyncio
from pathlib import Path

from thomas_ragx.worker.run import heartbeat


async def test_heartbeat_ticks_while_a_job_runs(tmp_path: Path) -> None:
    beat = tmp_path / "beat"
    stop = asyncio.Event()
    task = asyncio.create_task(heartbeat(beat, 0.01, stop))
    await asyncio.sleep(0.02)
    first = beat.stat().st_mtime_ns
    await asyncio.sleep(0.05)  # a "long job" elsewhere on the loop
    stop.set()
    await task
    assert beat.stat().st_mtime_ns > first
