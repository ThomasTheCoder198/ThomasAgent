from collections.abc import Sequence

import pytest

from tests.rag_fakes import HashEmbedder
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.embedding_cache import CachedEmbedder
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.registry.model import ModelHandle

pytestmark = pytest.mark.integration


async def _handle(pool: Pool):  # type: ignore[no-untyped-def]
    service = create_registry_service(Config(minio_access_key="a", minio_secret_key="s"), pool)
    model = await seed_dev_registry(service)
    return await service.model_handle(model.id), model.embedding_dims


async def test_cached_embedder_embeds_each_text_once_per_model(pool: Pool) -> None:
    handle, dims = await _handle(pool)
    fake = HashEmbedder(dims)
    cache = CachedEmbedder(pool, fake, batch_size=2)
    first = await cache.embed(handle, dims, ["a b", "c d", "e f"])
    second = await cache.embed(handle, dims, ["c d", "a b", "g h"])
    assert (first.hits, first.misses, second.hits, second.misses) == (0, 3, 2, 1)
    assert fake.calls == [["a b", "c d"], ["e f"], ["g h"]]  # batches of two, cache hits skipped
    assert second.vectors[1] == pytest.approx(first.vectors[0], rel=1e-6)  # float32 round trip


async def test_dims_mismatch_is_a_non_retryable_ingest_error(pool: Pool) -> None:
    handle, dims = await _handle(pool)
    cache = CachedEmbedder(pool, HashEmbedder(dims, vector_length=dims - 1), batch_size=10)
    with pytest.raises(AppError) as caught:
        await cache.embed(handle, dims, ["x"])
    assert caught.value.code is ErrorCode.INGEST_EMBEDDING_MISMATCH and not caught.value.retryable()


async def test_provider_vector_count_mismatch_is_a_catalog_error(pool: Pool) -> None:
    handle, dims = await _handle(pool)

    class MissingVectors:
        async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
            return []

    with pytest.raises(AppError) as caught:
        await CachedEmbedder(pool, MissingVectors(), batch_size=2).embed(handle, dims, ["missing result"])
    assert caught.value.code is ErrorCode.INGEST_EMBEDDING_MISMATCH


async def test_warm_cache_rejects_a_different_kb_dimension(pool: Pool) -> None:
    handle, dims = await _handle(pool)
    fake = HashEmbedder(dims)
    cache = CachedEmbedder(pool, fake, batch_size=2)
    await cache.embed(handle, dims, ["warm dimension"])
    with pytest.raises(AppError) as caught:
        await cache.embed(handle, dims - 1, ["warm dimension"])
    assert caught.value.code is ErrorCode.INGEST_EMBEDDING_MISMATCH
    assert not caught.value.retryable()
    assert fake.calls == [["warm dimension"]]
