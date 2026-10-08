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
