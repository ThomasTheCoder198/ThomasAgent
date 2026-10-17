import uuid
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from typing import Final
from urllib.parse import urlsplit
from uuid import UUID

from psycopg import errors as pg_errors
from pydantic import SecretStr

from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry import repository
from thomas_ragx.registry.listing import ModelListing
from thomas_ragx.registry.model import (
    KIND_RULES,
    ROLE_CAPABILITIES,
    BaseUrlRule,
    Capability,
    ModelFacts,
    ModelHandle,
    ModelRecord,
    Provider,
    ProviderAccess,
    ProviderKind,
    Role,
)
from thomas_ragx.registry.model_check import ModelChecker
from thomas_ragx.registry.repository import Connection
from thomas_ragx.registry.vault import Sealed, Vault, provider_key_aad

NAME_MAX_LENGTH: Final = 200
HTTPS_SCHEME: Final = "https"
HTTP_SCHEME: Final = "http"


def _field_error(field: str, code: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": code}]})


def _clean_name(name: str) -> str:
    cleaned = name.strip()
    if not cleaned or len(cleaned) > NAME_MAX_LENGTH:
        raise _field_error("name", "INVALID")
    return cleaned


def _clean_base_url(kind: ProviderKind, base_url: str) -> str:
    """Empty means the provider default. A built-in kind takes only https; a self-hosted one is trusted with
    http and private hosts (host.docker.internal), so there is no DNS or IP check here."""
    cleaned = base_url.strip().rstrip("/")
    if not cleaned:
        return cleaned
    schemes = (HTTPS_SCHEME,) if KIND_RULES[kind].https_only else (HTTP_SCHEME, HTTPS_SCHEME)
    try:
        parts = urlsplit(cleaned)
        parts.port  # noqa: B018 - reading it validates the port
    except ValueError:
        raise _field_error("baseUrl", "INVALID") from None
    # userinfo would smuggle credentials into a URL that is stored and shown in clear.
    if (
        parts.scheme not in schemes
        or not parts.hostname
        or "@" in parts.netloc
        or any(char.isspace() for char in cleaned)
    ):
        raise _field_error("baseUrl", "INVALID")
    return cleaned


def _provider_field_errors(
    kind: ProviderKind, base_url: str, *, key_missing: bool, check_key: bool
) -> list[dict[str, str]]:
    """Which provider fields the kind demands; an update skips the key so keyless old rows stay fixable."""
    rules = KIND_RULES[kind]
    errors: list[dict[str, str]] = []
    if check_key and rules.api_key_required and key_missing:
        errors.append({"field": "apiKey", "code": "REQUIRED"})
    if rules.base_url is BaseUrlRule.REQUIRED and not base_url:
        errors.append({"field": "baseUrl", "code": "REQUIRED"})
    return errors


def _require_provider_fields(errors: list[dict[str, str]]) -> None:
    if errors:
        raise AppError(ErrorCode.VALIDATION_FAILED, details={"fields": errors})


def _require_role_fit(role: Role, model: ModelRecord) -> None:
    if not model.capabilities & ROLE_CAPABILITIES[role]:
        raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)


def _needs_measuring(model: ModelRecord) -> bool:
    # A KB copies its dims at creation, so an embedding model must know its size before it is used.
    return Capability.EMBEDDING in model.capabilities and not model.embedding_dims


class LLMProviderService:
    def __init__(
        self,
        pool: Pool,
        vault: Vault,
        listing: ModelListing,
        checker: ModelChecker,
        describe: Callable[[ProviderKind, str], ModelFacts],
    ) -> None:
        self._pool = pool
        self._vault = vault
        self._listing = listing
        self._checker = checker
        self._describe = describe

    @asynccontextmanager
    async def _transaction(self) -> AsyncIterator[Connection]:
        async with self._pool.connection() as conn, conn.transaction():
            yield conn

    def _seal(self, provider_id: UUID, api_key: SecretStr | None) -> Sealed | None:
        if api_key is None or not api_key.get_secret_value():
            return None
        return self._vault.seal(api_key.get_secret_value(), provider_key_aad(provider_id))

    async def _access(self, conn: Connection, provider_id: UUID) -> ProviderAccess:
        row = await repository.get_provider_row(conn, provider_id)
        if row is None:
            raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
        key = None
        if row["api_key_ciphertext"] is not None:
            sealed = Sealed(nonce=bytes(row["api_key_nonce"]), ciphertext=bytes(row["api_key_ciphertext"]))
            key = self._vault.open(sealed, provider_key_aad(provider_id))
        return ProviderAccess(kind=ProviderKind(row["kind"]), base_url=row["base_url"], api_key=key)

    async def create_provider(
        self, kind: ProviderKind, name: str, base_url: str, api_key: SecretStr | None
    ) -> Provider:
        provider_id = uuid.uuid4()
        cleaned_name, cleaned_url = _clean_name(name), _clean_base_url(kind, base_url)
        sealed = self._seal(provider_id, api_key)
        _require_provider_fields(
            _provider_field_errors(kind, cleaned_url, key_missing=sealed is None, check_key=True)
        )
        try:
            async with self._transaction() as conn:
                return await repository.insert_provider(
                    conn, provider_id, kind, cleaned_name, cleaned_url, sealed
                )
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.LLM_NAME_TAKEN) from None

    async def update_provider(
        self, provider_id: UUID, name: str, base_url: str, api_key: SecretStr | None, clear_api_key: bool
    ) -> Provider:
        cleaned_name = _clean_name(name)
        try:
            async with self._transaction() as conn:
                # Lock order is provider, then its models, everywhere. Locking first makes the URL compared
                # below the committed one, so a concurrent edit cannot leave this URL with that edit key.
                existing = await repository.get_provider_row_for_update(conn, provider_id)
                if existing is None:
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
                kind = ProviderKind(existing["kind"])
                cleaned_url = _clean_base_url(kind, base_url)
                _require_provider_fields(
                    _provider_field_errors(kind, cleaned_url, key_missing=False, check_key=False)
                )
                updated = await repository.update_provider(conn, provider_id, cleaned_name, cleaned_url)
                if updated is None:
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
                sealed = self._seal(provider_id, api_key)
                if clear_api_key:
                    await repository.set_provider_key(conn, provider_id, None)
                elif sealed is not None:
                    await repository.set_provider_key(conn, provider_id, sealed)
                elif cleaned_url != existing["base_url"]:
                    # A key belongs to the address it was entered for; it must not follow a new host.
                    await repository.set_provider_key(conn, provider_id, None)
                row = await repository.get_provider_row(conn, provider_id)
                assert row is not None  # noqa: S101 - same transaction as the update
                return repository.to_provider(row)
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.LLM_NAME_TAKEN) from None

    async def delete_provider(self, provider_id: UUID) -> None:
        try:
            async with self._transaction() as conn:
                # Provider first, then its models, like every other path.
                if await repository.get_provider_row_for_update(conn, provider_id) is None:
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
                if not await repository.delete_provider(conn, provider_id):
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
        except (pg_errors.ForeignKeyViolation, pg_errors.RestrictViolation):
            raise AppError(ErrorCode.LLM_MODEL_IN_USE) from None

    async def list_providers(self) -> list[Provider]:
        async with self._pool.connection() as conn:
            return await repository.list_providers(conn)

    async def sync_models(self, provider_id: UUID) -> int:
        async with self._pool.connection() as conn:
            access = await self._access(conn, provider_id)
        listed = {model.name: model for model in await self._listing.list_models(access)}
        async with self._transaction() as conn:
            await self._lock_provider_unchanged(conn, provider_id, access)
            for name in sorted(listed):
                # A list that carries facts is the authority; LiteLLM's bundled map only fills a bare name.
                facts = listed[name].facts or self._describe(access.kind, name)
                await repository.upsert_synced_model(conn, uuid.uuid4(), provider_id, name, facts)
        return len(listed)

    async def add_manual_model(
        self,
        provider_id: UUID,
        name: str,
        capabilities: frozenset[Capability],
        embedding_dims: int | None,
        context_window: int | None,
    ) -> ModelRecord:
        try:
            async with self._transaction() as conn:
                if await repository.get_provider_row(conn, provider_id) is None:
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
                return await repository.insert_manual_model(
                    conn,
                    uuid.uuid4(),
                    provider_id,
                    _clean_name(name),
                    capabilities,
                    embedding_dims,
                    context_window,
                )
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.LLM_NAME_TAKEN) from None

    async def update_model(self, model_id: UUID, capabilities: frozenset[Capability]) -> ModelRecord:
        async with self._transaction() as conn:
            updated = await repository.update_model(conn, model_id, capabilities)
        if updated is None:
            raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
        return updated

    async def delete_model(self, model_id: UUID) -> None:
        try:
            async with self._transaction() as conn:
                if not await repository.delete_model(conn, model_id):
                    raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
        except (pg_errors.ForeignKeyViolation, pg_errors.RestrictViolation):
            raise AppError(ErrorCode.LLM_MODEL_IN_USE) from None

    async def list_models(self, provider_id: UUID | None = None) -> list[ModelRecord]:
        async with self._pool.connection() as conn:
            return await repository.list_models(conn, provider_id)

    async def test_model(self, model_id: UUID) -> None:
        async with self._pool.connection() as conn:
            model = await repository.get_model(conn, model_id)
            if model is None:
                raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
            access = await self._access(conn, model.provider_id)
        if not _needs_measuring(model):
            await self._checker.check(access, model)
            # A success only counts for the address it was made against.
            async with self._transaction() as conn:
                await self._lock_provider_unchanged(conn, model.provider_id, access)
            return
        # The probe call is the test, so one provider call both proves the model works and sizes it.
        dims = await self._checker.measure_embedding_dims(access, model)
        async with self._transaction() as conn:
            await self._lock_provider_unchanged(conn, model.provider_id, access)
            await repository.set_measured_dims(conn, model_id, dims)

    async def _lock_provider_unchanged(
        self, conn: Connection, provider_id: UUID, called: ProviderAccess
    ) -> None:
        """A provider call's result applies only to the address it went to: lock the provider and compare."""
        row = await repository.get_provider_row_for_update(conn, provider_id)
        if row is None or row["base_url"] != called.base_url:
            raise AppError(ErrorCode.LLM_PROVIDER_REJECTED)

    async def assign_role(self, role: Role, model_id: UUID) -> None:
        # Provider calls run before the transaction, like sync_models.
        async with self._pool.connection() as conn:
            model = await repository.get_model(conn, model_id)
            if model is None:
                raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
            _require_role_fit(role, model)
            access = (
                await self._access(conn, model.provider_id)
                if role is Role.EMBEDDING and _needs_measuring(model)
                else None
            )
        probed = await self._checker.measure_embedding_dims(access, model) if access else None
        async with self._transaction() as conn:
            # Provider, then model: the one lock order of every path, so an address edit waits for this
            # commit (the assignment then reflects the probed address) instead of racing it or deadlocking.
            if access is not None:
                await self._lock_provider_unchanged(conn, model.provider_id, access)
            # The probe can take seconds: judge the model as it is now, not as it was.
            fresh = await repository.lock_model(conn, model_id)
            if fresh is None:
                raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
            _require_role_fit(role, fresh)
            if probed is not None:
                if fresh.embedding_dims is None:
                    if not await repository.set_measured_dims(conn, model_id, probed):
                        raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)
                elif fresh.embedding_dims != probed:
                    # A competing probe or a KB fixed another size; the vectors would not fit.
                    raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)
            elif role is Role.EMBEDDING and not fresh.embedding_dims:
                raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)
            await repository.assign_role(conn, role, model_id)

    async def clear_role(self, role: Role) -> None:
        async with self._transaction() as conn:
            await repository.clear_role(conn, role)

    async def roles(self) -> dict[Role, ModelRecord | None]:
        async with self._pool.connection() as conn:
            assigned = await repository.role_assignments(conn)
        return {role: assigned.get(role) for role in Role}

    async def role_model(self, conn: Connection, role: Role) -> ModelRecord:
        model = (await repository.role_assignments(conn)).get(role)
        if model is None:
            raise AppError(ErrorCode.LLM_ROLE_NOT_ASSIGNED)
        return model

    async def model_handle(self, model_id: UUID) -> ModelHandle:
        async with self._pool.connection() as conn:
            model = await repository.get_model(conn, model_id)
            if model is None:
                raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
            return ModelHandle(model, await self._access(conn, model.provider_id))

    async def role_handle(self, role: Role) -> ModelHandle:
        async with self._pool.connection() as conn:
            model = await self.role_model(conn, role)
            return ModelHandle(model, await self._access(conn, model.provider_id))
