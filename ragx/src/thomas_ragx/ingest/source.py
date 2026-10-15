from pathlib import Path, PurePosixPath

from thomas_ragx.ingest.repository import IngestTarget
from thomas_ragx.kb.storage import RAW_BUCKET, ObjectStorage, raw_key
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError


def extension_of(target: IngestTarget) -> str:
    return PurePosixPath(target.name).suffix.lower()


async def fetch_verified(storage: ObjectStorage, target: IngestTarget, work_dir: Path) -> Path:
    """Verify stored bytes because the presigned ticket remains usable after confirm."""
    work_dir.mkdir(parents=True, exist_ok=True)
    # The extension stays: Datalab's SDK picks the MIME type from the file name. The attempt id keeps two
    # attempts of one file in one process from sharing (and unlinking) one copy.
    path = work_dir / f"{target.file_id}-{target.version}-{target.attempt_id}{extension_of(target)}"
    digest = await storage.download_to(RAW_BUCKET, raw_key(target.org_id, target.kb_id, target.file_id), path)
    if digest is None:
        path.unlink(missing_ok=True)
        raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
    if digest != target.sha256:
        path.unlink(missing_ok=True)
        raise AppError(ErrorCode.INGEST_CONTENT_CHANGED)
    return path
