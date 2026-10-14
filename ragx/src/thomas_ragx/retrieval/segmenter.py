from typing import Final, Protocol

SEGMENTER_UNDERTHESEA: Final = "underthesea"
SEGMENTER_NONE: Final = "none"
UNDERTHESEA_TEXT_FORMAT: Final = "text"


class Segmenter(Protocol):
    def segment(self, text: str) -> str: ...


class IdentitySegmenter:
    def segment(self, text: str) -> str:
        return text


class UndertheseaSegmenter:
    """Joins Vietnamese compounds with "_" (hợp_đồng) so BM25 matches words, not syllables (spike S6)."""

    def segment(self, text: str) -> str:
        from underthesea import word_tokenize

        return str(word_tokenize(text, format=UNDERTHESEA_TEXT_FORMAT))


def create_segmenter(name: str) -> Segmenter:
    if name == SEGMENTER_UNDERTHESEA:
        return UndertheseaSegmenter()
    if name == SEGMENTER_NONE:
        return IdentitySegmenter()
    raise ValueError(f"unknown RAG_VI_TOKENIZER {name!r}")
