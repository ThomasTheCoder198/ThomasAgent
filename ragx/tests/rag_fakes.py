import hashlib
import math
from collections.abc import Sequence

from thomas_ragx.registry.model import ModelHandle

HASH_BYTES = 4


def _bucket(token: str, dims: int) -> int:
    return int.from_bytes(hashlib.sha256(token.encode()).digest()[:HASH_BYTES], "big") % dims


class HashEmbedder:
    """Bag-of-words vectors: texts sharing words are close, so dense search means something in tests."""

    def __init__(self, dims: int, vector_length: int | None = None) -> None:
        self.dims = dims
        self.vector_length = vector_length or dims
        self.calls: list[list[str]] = []

    async def embed(self, model: ModelHandle, texts: Sequence[str]) -> list[list[float]]:
        self.calls.append(list(texts))
        vectors = []
        for text in texts:
            vector = [0.0] * self.vector_length
            for token in text.lower().split():
                vector[_bucket(token, self.vector_length)] += 1.0
            norm = math.sqrt(sum(v * v for v in vector)) or 1.0
            vectors.append([v / norm for v in vector])
        return vectors
