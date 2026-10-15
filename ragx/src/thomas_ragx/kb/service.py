import uuid
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from uuid import UUID

from psycopg import errors as pg_errors

from thomas_ragx.auth.caller import Caller
from thomas_ragx.db.outbox import PURGE_KIND_KB, STREAM_KB_PURGE_REQUESTED, enqueue_outbox
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import repository
from thomas_ragx.kb.model import DEFAULT_KB_NAME, KnowledgeBase, Org
from thomas_ragx.kb.repository import Connection
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import Role
from thomas_ragx.registry.service import LLMProviderService


async def require_org(conn: Connection, org_id: str) -> None:
    if not await repository.org_exists(conn, org_id):
        raise AppError(ErrorCode.ORG_NOT_PROVISIONED)


class KnowledgeBaseService:
    def __init__(self, pool: Pool, registry: LLMProviderService) -> None:
        self._pool = pool
        self._registry = registry

    @asynccontextmanager
    async def _transaction(self) -> AsyncIterator[Connection]:
        async with self._pool.connection() as conn, conn.transaction():
            yield conn

    async def provision(
        self, caller: Caller, org_id: str, name: str, external_parsing: bool, settings_version: int
    ) -> Org:
        if org_id != caller.org_id:
            raise AppError(ErrorCode.FORBIDDEN)
        async with self._transaction() as conn:
            await repository.upsert_org(conn, org_id, name, external_parsing, settings_version)
            # The row lock serializes concurrent first provisions; the partial unique index is the backstop.
            org_row = await repository.lock_org(conn, org_id)
            if org_row is None:
                raise AppError(ErrorCode.INTERNAL_ERROR)
            default_id = await repository.default_kb_id(conn, org_id)
            if default_id is None:
                model = await self._registry.role_model(conn, Role.EMBEDDING)
                default_id = uuid.uuid4()
                await repository.insert_kb(conn, default_id, org_id, DEFAULT_KB_NAME, True, model)
            return repository.to_org(org_row, default_id)

    async def list_all(self, caller: Caller) -> list[KnowledgeBase]:
        async with self._pool.connection() as conn:
            await require_org(conn, caller.org_id)
            return await repository.list_kbs(conn, caller.org_id)

    async def create(self, caller: Caller, name: str) -> KnowledgeBase:
        kb_id = uuid.uuid4()
        try:
            async with self._transaction() as conn:
                await require_org(conn, caller.org_id)
                model = await self._registry.role_model(conn, Role.EMBEDDING)
                await repository.insert_kb(conn, kb_id, caller.org_id, name, False, model)
                created = await repository.get_kb(conn, caller.org_id, kb_id)
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.CONFLICT) from None
        if created is None:
            raise AppError(ErrorCode.INTERNAL_ERROR)
        return created

    async def get(self, caller: Caller, kb_id: UUID) -> KnowledgeBase:
        async with self._pool.connection() as conn:
            await require_org(conn, caller.org_id)
            kb = await repository.get_kb(conn, caller.org_id, kb_id)
        if kb is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)
        return kb

    async def rename(self, caller: Caller, kb_id: UUID, name: str) -> KnowledgeBase:
        try:
            async with self._transaction() as conn:
                await require_org(conn, caller.org_id)
                if not await repository.rename_kb(conn, caller.org_id, kb_id, name):
                    raise AppError(ErrorCode.KB_NOT_FOUND)
                renamed = await repository.get_kb(conn, caller.org_id, kb_id)
        except pg_errors.UniqueViolation:
            raise AppError(ErrorCode.CONFLICT) from None
        if renamed is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)
        return renamed

    async def delete(self, caller: Caller, kb_id: UUID, request_id: str) -> None:
        async with self._transaction() as conn:
            await require_org(conn, caller.org_id)
            kb = await repository.lock_kb(conn, caller.org_id, kb_id)
            if kb is None:
                raise AppError(ErrorCode.KB_NOT_FOUND)
            if kb.is_default:
                raise AppError(ErrorCode.CONFLICT)
            await repository.mark_kb_deleted(conn, caller.org_id, kb_id)
            await enqueue_outbox(
                conn,
                caller.org_id,
                STREAM_KB_PURGE_REQUESTED,
                {"kind": PURGE_KIND_KB, "kb_id": str(kb_id)},
                request_id,
            )
