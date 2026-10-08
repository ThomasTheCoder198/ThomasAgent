import re
from pathlib import Path

import yaml

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config

CONTRACT = Path(__file__).resolve().parents[3] / "contracts" / "openapi" / "ragx.v1.yaml"
HTTP_METHODS = {"get", "put", "post", "patch", "delete"}
PARAMETER = re.compile(r"\{[^}]+\}")


def _normalize(path: str) -> str:
    return PARAMETER.sub("{}", path)


def test_every_documented_operation_is_served_and_every_served_route_is_documented() -> None:
    document = yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))
    documented = {
        (method.upper(), _normalize(path))
        for path, item in document["paths"].items()
        for method in item
        if method in HTTP_METHODS
    }
    app = create_app(
        Config(minio_access_key="a", minio_secret_key="s", otlp_endpoint=""), resources=FakeResources()
    )
    served = {
        (method.upper(), _normalize(path))
        for path, item in app.openapi()["paths"].items()
        for method in item
        if method in HTTP_METHODS
    }
    assert served == documented
