import re
from collections.abc import Iterable, Iterator
from pathlib import Path

import yaml
from fastapi.routing import APIRoute
from starlette.routing import BaseRoute

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config

CONTRACT = Path(__file__).resolve().parents[3] / "contracts" / "openapi" / "ragx.v1.yaml"
HTTP_METHODS = {"get", "put", "post", "patch", "delete"}
PARAMETER = re.compile(r"\{[^}]+\}")
CONTRACT_PREFIXES = ("/v1/", "/healthz", "/readyz")


def _normalize(path: str) -> str:
    return PARAMETER.sub("{}", path)


def _api_routes(routes: Iterable[BaseRoute]) -> Iterator[APIRoute]:
    """FastAPI 0.142 wraps each include_router() in a lazy route; follow it to reach the endpoints.

    If an upgrade renames that attribute, the walk finds nothing and the equality check below fails loudly.
    """
    for route in routes:
        if isinstance(route, APIRoute):
            yield route
        included = getattr(route, "original_router", None)
        if included is not None:
            yield from _api_routes(included.routes)


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
    # Walk the routes too: a /v1 route hidden from the generated schema must not escape the contract.
    routed = {
        (method.upper(), _normalize(route.path))
        for route in _api_routes(app.routes)
        if route.path.startswith(CONTRACT_PREFIXES)
        for method in route.methods
        if method.lower() in HTTP_METHODS
    }
    assert served == documented
    assert routed == documented
