import os
import subprocess
import sys

SCRIPT = (
    "import uuid; from thomas_ragx.ingest.chunker import chunk_id_for, point_id_for; "
    "print(point_id_for(chunk_id_for(uuid.UUID(int=7), 3, 11)))"
)


def _run(seed: str) -> str:
    env = {**os.environ, "PYTHONHASHSEED": seed}
    return subprocess.run(  # noqa: S603 - fixed Python script and interpreter; only hash seed varies.
        [sys.executable, "-c", SCRIPT], env=env, capture_output=True, text=True, check=True
    ).stdout


def test_point_ids_do_not_depend_on_the_hash_seed() -> None:
    assert _run("1") == _run("2") != ""
