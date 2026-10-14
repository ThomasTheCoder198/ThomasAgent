from fastapi.testclient import TestClient

from tests.support import FakeResources
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import Config


def _config() -> Config:
    return Config(minio_access_key="a", minio_secret_key="s", otlp_endpoint="")


def test_readyz_reports_ready_when_every_dependency_answers() -> None:
    resources = FakeResources()
    with TestClient(create_app(_config(), resources=resources)) as client:
        response = client.get("/readyz")
    assert response.status_code == 200
    assert response.json()["data"] == {"status": "ready"}
    assert resources.opened and resources.closed
    assert set(resources.ready) == {"postgres", "redis", "storage", "qdrant"}


def test_readyz_is_503_without_naming_internals_when_a_dependency_is_down() -> None:
    resources = FakeResources({"postgres": True, "redis": False, "storage": True})
    with TestClient(create_app(_config(), resources=resources)) as client:
        response = client.get("/readyz")
    assert response.status_code == 503
    assert response.json()["error"]["code"] == "INTERNAL_ERROR"
    assert "redis" not in response.text


def test_healthz_needs_no_dependency() -> None:
    resources = FakeResources({"postgres": False, "redis": False, "storage": False})
    with TestClient(create_app(_config(), resources=resources)) as client:
        assert client.get("/healthz").status_code == 200


def test_config_has_no_core_callback_settings() -> None:
    fields = Config.model_fields
    assert "core_internal_url" not in fields
    assert "core_service_token" not in fields


def test_readyz_is_unavailable_when_qdrant_is_down() -> None:
    resources = FakeResources({"postgres": True, "redis": True, "storage": True, "qdrant": False})
    with TestClient(create_app(_config(), resources=resources)) as client:
        response = client.get("/readyz")
    assert response.status_code == 503
    assert response.json()["error"]["code"] == "INTERNAL_ERROR"
    assert "qdrant" not in response.text
