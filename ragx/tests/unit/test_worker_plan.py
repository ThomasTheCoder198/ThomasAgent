from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.platform.config import Config
from thomas_ragx.worker.plan import consumer_plan


def test_consumer_plan_runs_one_ingest_consumer_per_slot_with_the_long_visibility() -> None:
    config = Config(minio_access_key="a", minio_secret_key="s", ingest_concurrency=3)
    plan = consumer_plan(config, "host-1")
    ingest = [spec for spec in plan if spec.stream == STREAM_INGEST_REQUESTED]
    purge = [spec for spec in plan if spec.stream == STREAM_KB_PURGE_REQUESTED]
    assert len(ingest) == 3 and len(purge) == 1
    assert len({spec.consumer_name for spec in plan}) == len(plan)
    assert {spec.visibility_timeout_ms for spec in ingest} == {config.ingest_visibility_timeout_milliseconds}
    assert purge[0].visibility_timeout_ms == config.stream_visibility_timeout_milliseconds
    assert {spec.batch_size for spec in ingest} == {1}
    assert purge[0].batch_size == config.stream_batch_size
