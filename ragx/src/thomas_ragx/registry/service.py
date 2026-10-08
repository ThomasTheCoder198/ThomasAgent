import uuid
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from typing import Final
from uuid import UUID

from psycopg import errors as pg_errors
from pydantic import SecretStr

from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry import repository
from thomas_ragx.registry.listing import ModelListing
from thomas_ragx.registry.model import (
    ROLE_CAPABILITIES,
    Capability,
    ModelFacts,
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
URL_SCHEMES: Final = ("http://", "https://")


def _field_error(field: str, code: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": code}]})


def _clean_name(name: str) -> str:
    cleaned = name.strip()
    if not cleaned or len(cleaned) > NAME_MAX_LENGTH:
        raise _field_error("name", "INVALID")
    return cleaned


def _clean_base_url(base_url: str) -> str:
    cleaned = base_url.strip().rstrip("/")
    if cleaned and not cleaned.startswith(URL_SCHEMES):
        raise _field_error("baseUrl", "INVALID")
    return cleaned


def _require_role_fit(role: Role, model: ModelRecord) -> None:
    if not model.capabilities & ROLE_CAPABILITIES[role]:
        raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)
    if role is Role.EMBEDDING and not model.embedding_dims:
        # A KB copies its dims at creation; an embedding model without dims could never build a collection.
        raise AppError(ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH)


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
        try:
            async with self._transaction() as conn:
                return await repository.insert_provider(
                    conn,
                    provider_id,
                    kind,
                    _clean_name(name),
                    _clean_base_url(base_url),
                    self._seal(provider_id, api_key),
                )
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.LLM_NAME_TAKEN) from None

    async def update_provider(
        self, provider_id: UUID, name: str, base_url: str, api_key: SecretStr | None, clear_api_key: bool
    ) -> Provider:
        try:
            async with self._transaction() as conn:
                updated = await repository.update_provider(
                    conn, provider_id, _clean_name(name), _clean_base_url(base_url)
                )
                if updated is None:
                    raise AppError(ErrorCode.LLM_PROVIDER_NOT_FOUND)
                if clear_api_key or api_key is not None:
                    await repository.set_provider_key(
                        conn, provider_id, None if clear_api_key else self._seal(provider_id, api_key)
                    )
                row = await repository.get_provider_row(conn, provider_id)
                assert row is not None  # noqa: S101 - same transaction as the update
                return repository.to_provider(row)
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.LLM_NAME_TAKEN) from None

    async def delete_provider(self, provider_id: UUID) -> None:
        try:
            async with self._transaction() as conn:
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
        names = sorted(set(await self._listing.list_models(access)))
        async with self._transaction() as conn:
            for name in names:
                await repository.upsert_synced_model(
                    conn, uuid.uuid4(), provider_id, name, self._describe(access.kind, name)
                )
        return len(names)

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

    async def update_model(
        self, model_id: UUID, capabilities: frozenset[Capability], embedding_dims: int | None
    ) -> ModelRecord:
        async with self._transaction() as conn:
            updated = await repository.update_model(conn, model_id, capabilities, embedding_dims)
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
        await self._checker.check(access, model)

    async def assign_role(self, role: Role, model_id: UUID) -> None:
        async with self._transaction() as conn:
            model = await repository.get_model(conn, model_id)
            if model is None:
                raise AppError(ErrorCode.LLM_MODEL_NOT_FOUND)
            _require_role_fit(role, model)
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
