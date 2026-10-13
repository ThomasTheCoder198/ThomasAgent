from typing import Any

from minio import Minio
from opentelemetry.sdk.trace import TracerProvider

from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.parsing.registry import local_registry
from thomas_ragx.parsing.service import ParseService


def _pipeline(pool: Pool, minio: Minio, tmp_path: Any) -> IngestPipeline:
    tracer = TracerProvider().get_tracer(__name__)
    parser = ParseService(
        local_registry(),
        None,
        mode="balanced",
        retry_mode="accurate",
        min_quality=3.0,
        tracer=tracer,
        local_first_extensions=frozenset({".docx", ".pptx"}),
    )
    return IngestPipeline(pool, ObjectStorage(minio), parser, tracer, work_dir=tmp_path, lease_seconds=60.0)
