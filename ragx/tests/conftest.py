import base64
import sys

import pytest

if sys.platform == "win32":
    import asyncio

    @pytest.fixture(scope="session")
    def event_loop_policy() -> asyncio.AbstractEventLoopPolicy:
        # Psycopg async requires a selector loop on Windows.
        return asyncio.WindowsSelectorEventLoopPolicy()


TEST_VAULT_MASTER_KEY = base64.b64encode(bytes(range(32))).decode()


@pytest.fixture(autouse=True)
def required_settings(monkeypatch: pytest.MonkeyPatch) -> None:
    """Required secrets have no YAML default; unit tests never reach these endpoints."""
    monkeypatch.setenv("RAG_DATABASE_URL", "postgresql://ragx:ragx@127.0.0.1:1/ragx")
    monkeypatch.setenv("RAG_VAULT_MASTER_KEY", TEST_VAULT_MASTER_KEY)
    monkeypatch.setenv("RAG_CORE_PUBLIC_KEYS", "{}")
    monkeypatch.setenv("LITELLM_LOCAL_MODEL_COST_MAP", "True")


@pytest.fixture(autouse=True)
def close_api_metrics(monkeypatch: pytest.MonkeyPatch):  # type: ignore[no-untyped-def]
    # ASGITransport and bare TestClient calls do not run the app lifespan.
    from thomas_ragx.api import app as api_app

    configure = api_app.configure_worker_metrics
    cleanup = []
    closed = set()

    def tracked(config):  # type: ignore[no-untyped-def]
        provider, recorder = configure(config)
        shutdown = provider.shutdown

        def close() -> None:
            if id(provider) not in closed:
                closed.add(id(provider))
                shutdown()

        monkeypatch.setattr(provider, "shutdown", close)
        cleanup.append(close)
        return provider, recorder

    monkeypatch.setattr(api_app, "configure_worker_metrics", tracked)
    yield
    for close in cleanup:
        close()
