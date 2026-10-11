import json
from pathlib import Path

from thomas_ragx.kb.upload import SUPPORTED_EXTENSIONS
from thomas_ragx.platform.config import Config

CONTRACT = Path(__file__).resolve().parents[3] / "contracts" / "kb-upload.json"


def test_upload_limits_match_the_shared_contract() -> None:
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    config = Config(minio_access_key="a", minio_secret_key="s")
    assert list(SUPPORTED_EXTENSIONS) == contract["extensions"]
    assert config.upload_max_bytes == contract["maxFileBytes"]
