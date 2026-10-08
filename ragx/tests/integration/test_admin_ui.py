import re
from typing import Any

import httpx
import pytest
from pydantic import SecretStr

from tests.integration.api_client import api_client
from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

pytestmark = pytest.mark.integration
EMAIL = "owner@example.com"
PASSWORD = "correct horse battery"  # noqa: S105 - test credential
CSRF = re.compile(r'name="csrf_token" value="([^"]+)"')
REQUEST_ID = re.compile(r"req_[A-Za-z0-9-]{8,64}")


async def _login(client: httpx.AsyncClient, password: str = PASSWORD) -> httpx.Response:
    return await client.post("/ui/login", data={"email": EMAIL, "password": password}, follow_redirects=False)


async def _csrf(client: httpx.AsyncClient, path: str) -> str:
    match = CSRF.search((await client.get(path)).text)
    assert match is not None
    return match.group(1)


@pytest.fixture
def ui_settings() -> dict[str, Any]:
    return {"ui_login_max_attempts": 3, "ui_login_window_seconds": 60}


async def test_admin_account_is_created_once_and_needs_a_long_password(pool: Pool, redis_url: str) -> None:
    async with api_client(pool, redis_url=redis_url) as (_, app):
        service = app.state.admin_service
        assert await service.ensure_admin(EMAIL, SecretStr(PASSWORD)) is True
        assert await service.ensure_admin(EMAIL, SecretStr("another long password")) is False
        with pytest.raises(AppError) as short:
            await service.ensure_admin("x@example.com", SecretStr("short"))
    assert short.value.code is ErrorCode.VALIDATION_FAILED


async def test_login_sets_a_strict_cookie_and_pages_need_it(
    pool: Pool, redis_url: str, ui_settings: dict[str, Any]
) -> None:
    async with api_client(pool, redis_url=redis_url, **ui_settings) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        anonymous = await client.get("/ui/providers", follow_redirects=False)
        assert (anonymous.status_code, anonymous.headers["location"]) == (303, "/ui/login")
        logged_in = await _login(client)
        assert logged_in.status_code == 303
        cookie = logged_in.headers["set-cookie"].lower()
        assert all(part in cookie for part in ("httponly", "samesite=lax", "path=/ui"))
        page = await client.get("/ui/providers")
        assert page.status_code == 200
        assert "default-src 'self'" in page.headers["content-security-policy"]


async def test_login_is_rate_limited_after_failures(
    pool: Pool, redis_url: str, ui_settings: dict[str, Any]
) -> None:
    async with api_client(pool, redis_url=redis_url, **ui_settings) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        for _ in range(ui_settings["ui_login_max_attempts"]):
            assert (await _login(client, "wrong password!")).status_code == 401
        blocked = await _login(client)
    assert blocked.status_code == 429


async def test_unsafe_requests_need_the_csrf_token(pool: Pool, redis_url: str) -> None:
    async with api_client(pool, redis_url=redis_url) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        await _login(client)
        forged = await client.post(
            "/ui/providers", data={"kind": "openrouter", "name": "x"}, follow_redirects=False
        )
    assert forged.status_code == 403


async def test_provider_key_is_never_rendered_and_roles_offer_only_capable_models(
    pool: Pool, redis_url: str
) -> None:
    secret = "sk-or-v1-never-render-me"  # noqa: S105 - must never appear in HTML
    async with api_client(pool, redis_url=redis_url) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        await _login(client)
        token = await _csrf(client, "/ui/providers")
        created = await client.post(
            "/ui/providers",
            data={
                "csrf_token": token,
                "kind": "openai_compatible",
                "name": "Local",
                "base_url": "http://127.0.0.1:9/v1",
                "api_key": secret,
            },
            follow_redirects=False,
        )
        assert created.status_code == 303
        providers = await client.get("/ui/providers")
        provider_id = (await app.state.registry_service.list_providers())[0].id
        await client.post(
            "/ui/models",
            data={
                "csrf_token": token,
                "provider_id": str(provider_id),
                "name": "embed-a",
                "capabilities": "embedding",
                "embedding_dims": "768",
            },
        )
        await client.post(
            "/ui/models",
            data={
                "csrf_token": token,
                "provider_id": str(provider_id),
                "name": "chat-a",
                "capabilities": "chat",
            },
        )
        roles = await client.get("/ui/roles")
    assert secret not in providers.text
    embedding_select = roles.text.split('name="role" value="embedding"')[1].split("</form>")[0]
    assert "embed-a" in embedding_select
    assert "chat-a" not in embedding_select


async def test_delete_confirms_in_a_dialog_and_a_refusal_shows_the_request_id(
    pool: Pool, redis_url: str
) -> None:
    # Owner direction 2026-10-08: destructive actions confirm in a native <dialog>; errors carry requestId.
    async with api_client(pool, redis_url=redis_url) as (client, app):
        await app.state.resources.redis.flushdb()
        await app.state.admin_service.ensure_admin(EMAIL, SecretStr(PASSWORD))
        await _login(client)
        token = await _csrf(client, "/ui/providers")
        await client.post("/ui/providers", data={"csrf_token": token, "kind": "ollama", "name": "Ollama"})
        provider_id = (await app.state.registry_service.list_providers())[0].id
        await client.post(
            "/ui/models",
            data={
                "csrf_token": token,
                "provider_id": str(provider_id),
                "name": "chat-b",
                "capabilities": "chat",
            },
        )
        model_id = (await app.state.registry_service.list_models())[0].id
        await client.post(
            "/ui/roles", data={"csrf_token": token, "role": "chat.fast", "model_id": str(model_id)}
        )
        page = await client.get("/ui/providers")
        refused = await client.post(
            f"/ui/providers/{provider_id}/delete", data={"csrf_token": token}, follow_redirects=True
        )
    assert f'action="/ui/providers/{provider_id}/delete"' in page.text.split("<dialog")[1]
    assert refused.status_code == 200
    error = refused.text.split("notice-error")[1].split("</p>")[0]
    # The id shown is the failing POST's (the one in the logs), not the follow-up GET's.
    assert refused.history[0].headers["x-request-id"] in error
    assert REQUEST_ID.search(error) is not None


async def test_static_assets_are_packaged(pool: Pool) -> None:
    async with api_client(pool) as (client, _):
        for path in ("/ui/static/ragx.css", "/ui/static/app.js", "/ui/static/vendor/htmx.min.js"):
            assert (await client.get(path)).status_code == 200, path
