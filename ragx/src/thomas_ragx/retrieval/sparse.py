import threading
from collections.abc import Sequence
from pathlib import Path
from typing import Final

from fastembed.sparse.bm25 import Bm25
from qdrant_client.models import SparseVector

from thomas_ragx.retrieval.segmenter import Segmenter

BM25_MODEL: Final = "Qdrant/bm25"
# Bm25 insists on a supported language; without a stemmer it changes nothing (ruling B12).
BM25_LANGUAGE: Final = "english"


class SparseEncoder:
    """BM25 term weights, IDF applied by Qdrant (Modifier.IDF). Built offline from an empty local model path.

    The model is built on first use: construction scans Unicode (about 1 s), and the API process creates this
    object at startup even when no tool is called.
    """

    def __init__(self, segmenter: Segmenter, model_dir: Path, *, k: float, b: float, avg_len: float) -> None:
        self._segmenter = segmenter
        self._model_dir = model_dir
        self._params = {"k": k, "b": b, "avg_len": avg_len}
        self._bm25: Bm25 | None = None
        self._lock = threading.Lock()  # encode runs in worker threads (asyncio.to_thread)

    def _model(self) -> Bm25:
        with self._lock:
            if self._bm25 is None:
                self._model_dir.mkdir(parents=True, exist_ok=True)
                self._bm25 = Bm25(
                    BM25_MODEL,
                    specific_model_path=str(self._model_dir),
                    disable_stemmer=True,
                    language=BM25_LANGUAGE,
                    k=self._params["k"],
                    b=self._params["b"],
                    avg_len=self._params["avg_len"],
                )
            return self._bm25

    def encode_passages(self, texts: Sequence[str]) -> list[SparseVector]:
        segmented = [self._segmenter.segment(text) for text in texts]
        return [
            SparseVector(indices=e.indices.tolist(), values=e.values.tolist())
            for e in self._model().passage_embed(segmented)
        ]

    def encode_query(self, text: str) -> SparseVector:
        embedding = next(iter(self._model().query_embed(self._segmenter.segment(text))))
        return SparseVector(indices=embedding.indices.tolist(), values=[float(v) for v in embedding.values])
