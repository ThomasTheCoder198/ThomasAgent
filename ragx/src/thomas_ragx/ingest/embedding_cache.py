import hashlib
from array import array
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final, Protocol

from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.providers.embedding import Embedder
from thomas_ragx.registry.model import ModelHandle

FLOAT32: Final = "f"  # little-endian on every supported host (x86-64, arm64)
RESULT_HIT: Final = "hit"
RESULT_MISS: Final = "miss"


class CacheRecorder(Protocol):
    def observe_embedding_cache(self, result: str, count: int) -> None: ...


@dataclass(frozen=True)
class EmbedResult:
    vectors: list[list[float]]
    hits: int
    misses: int


def content_key(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def _pack(vector: Sequence[float]) -> bytes:
    return array(FLOAT32, vector).tobytes()


def _unpack(data: bytes) -> list[float]:
    values = array(FLOAT32)
    values.frombytes(data)
    return values.tolist()


class CachedEmbedder:
    """Equal text under one model embeds once, across files and orgs (main spec §4.2 step 6, ruling B7)."""

    def __init__(
        self, pool: Pool, embedder: Embedder, *, batch_size: int, metrics: CacheRecorder | None = None
    ) -> None:
        self._pool = pool
        self._embedder = embedder
        self._batch = batch_size
        self._metrics = metrics

    async def embed(self, model: ModelHandle, dims: int, texts: Sequence[str]) -> EmbedResult:
        keys = [content_key(text) for text in texts]
        async with self._pool.connection() as conn:
            rows = await (
                await conn.execute(
                    b"SELECT content_sha256, dims, vector FROM embedding_cache "
                    b"WHERE model_id = %s AND content_sha256 = ANY(%s)",
                    (model.record.id, list(set(keys))),
                )
            ).fetchall()
        known = {row["content_sha256"]: _unpack(bytes(row["vector"])) for row in rows}
        if any(row["dims"] != dims for row in rows) or any(len(vector) != dims for vector in known.values()):
            raise AppError(ErrorCode.INGEST_EMBEDDING_MISMATCH)
        missing = list(dict.fromkeys(key for key in keys if key not in known))
        missing_keys = set(missing)
        text_of = dict(zip(keys, texts, strict=True))
        for start in range(0, len(missing), self._batch):
            batch = missing[start : start + self._batch]
            vectors = await self._embedder.embed(model, [text_of[key] for key in batch])
            if len(vectors) != len(batch) or any(len(vector) != dims for vector in vectors):
                raise AppError(ErrorCode.INGEST_EMBEDDING_MISMATCH)  # ruling B6
            async with self._pool.connection() as conn, conn.cursor() as cursor:
                await cursor.executemany(
                    b"INSERT INTO embedding_cache (model_id, content_sha256, dims, vector) "
                    b"VALUES (%s, %s, %s, %s) "
                    b"ON CONFLICT DO NOTHING",
                    [
                        (model.record.id, key, dims, _pack(vector))
                        for key, vector in zip(batch, vectors, strict=True)
                    ],
                )
            known.update(zip(batch, vectors, strict=True))
        hits = len(keys) - sum(1 for key in keys if key in missing_keys)
        if self._metrics is not None:
            self._metrics.observe_embedding_cache(RESULT_HIT, hits)
            self._metrics.observe_embedding_cache(RESULT_MISS, len(keys) - hits)
        return EmbedResult([known[key] for key in keys], hits, len(keys) - hits)
