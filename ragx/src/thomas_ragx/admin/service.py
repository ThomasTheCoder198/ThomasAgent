import hashlib
import secrets
from datetime import UTC, datetime, timedelta
from typing import Final

import structlog
from argon2 import PasswordHasher
from argon2.exceptions import InvalidHashError, VerificationError, VerifyMismatchError
from pydantic import SecretStr

from thomas_ragx.admin import repository
from thomas_ragx.admin.limiter import LoginLimiter
from thomas_ragx.admin.model import AdminSession
from thomas_ragx.db.pool import Pool, create_pool
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

ADMIN_PASSWORD_MIN_LENGTH: Final = 12
SESSION_TOKEN_BYTES: Final = 32
CSRF_TOKEN_BYTES: Final = 32
log = structlog.get_logger(__name__)


def _hash_token(token: str) -> bytes:
    return hashlib.sha256(token.encode()).digest()


def _normalize_email(email: str) -> str:
    return email.strip().lower()


async def create_admin(pool: Pool, hasher: PasswordHasher, email: str, password: SecretStr) -> bool:
    """Creates the admin once; returns False when the email already exists (the password is not changed)."""
    normalized = _normalize_email(email)
    if "@" not in normalized or len(password.get_secret_value()) < ADMIN_PASSWORD_MIN_LENGTH:
        raise AppError(ErrorCode.VALIDATION_FAILED)
    async with pool.connection() as conn:
        return await repository.insert_admin(conn, normalized, hasher.hash(password.get_secret_value()))


class AdminService:
    def __init__(self, pool: Pool, limiter: LoginLimiter, session_ttl_seconds: int) -> None:
        self._pool = pool
        self._limiter = limiter
        self._ttl = timedelta(seconds=session_ttl_seconds)
        self._hasher = PasswordHasher()
        # Verifying against a real hash for unknown emails keeps both failure paths equally slow.
        self._decoy_hash = self._hasher.hash(secrets.token_urlsafe(SESSION_TOKEN_BYTES))

    async def ensure_admin(self, email: str, password: SecretStr) -> bool:
        return await create_admin(self._pool, self._hasher, email, password)

    def _verify(self, password_hash: str, password: str) -> bool:
        try:
            return self._hasher.verify(password_hash, password)
        except (VerifyMismatchError, VerificationError, InvalidHashError):
            return False

    async def login(self, email: str, password: str, ip: str) -> tuple[str, AdminSession]:
        normalized = _normalize_email(email)
        await self._limiter.check(normalized, ip)
        async with self._pool.connection() as conn:
            admin = await repository.admin_by_email(conn, normalized)
        valid = self._verify(admin["password_hash"] if admin else self._decoy_hash, password)
        if admin is None or not valid:
            await self._limiter.record_failure(normalized, ip)
            raise AppError(ErrorCode.AUTH_INVALID_CREDENTIALS)
        await self._limiter.reset(normalized, ip)
        token, csrf = secrets.token_urlsafe(SESSION_TOKEN_BYTES), secrets.token_urlsafe(CSRF_TOKEN_BYTES)
        expires = datetime.now(UTC) + self._ttl
        async with self._pool.connection() as conn:
            await repository.insert_session(conn, _hash_token(token), admin["id"], csrf, expires)
        return token, AdminSession(admin["id"], normalized, csrf, expires)

    async def session(self, token: str) -> AdminSession | None:
        async with self._pool.connection() as conn:
            return await repository.session_by_hash(conn, _hash_token(token))

    async def logout(self, token: str) -> None:
        async with self._pool.connection() as conn:
            await repository.delete_session(conn, _hash_token(token))


async def ensure_admin_account(config: Config) -> None:
    """Bootstrap: create the single admin once from RAG_ADMIN_EMAIL / RAG_ADMIN_INITIAL_PASSWORD."""
    if not config.admin_email or config.admin_initial_password is None:
        log.warning("admin account not configured; set RAG_ADMIN_EMAIL and RAG_ADMIN_INITIAL_PASSWORD")
        return
    pool = create_pool(config.database_url.get_secret_value(), min_size=1, max_size=1)
    await pool.open(wait=True, timeout=config.database_connect_timeout_seconds)
    try:
        created = await create_admin(
            pool, PasswordHasher(), config.admin_email, config.admin_initial_password
        )
        log.info("admin account ready", created=created)
    finally:
        await pool.close()
