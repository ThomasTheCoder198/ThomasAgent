import statistics
from pathlib import Path
from typing import Any, Final

import pymupdf

from thomas_ragx.parsing.model import BlockCollector, BlockType, ParsedDocument, ParseProvider, pdf_bbox

TEXT_BLOCK: Final = 0
HEADING_SIZE_RATIO: Final = 1.25
TOP_HEADING_SIZE_RATIO: Final = 1.6
HEADING_MAX_WORDS: Final = 20
TOP_HEADING_LEVEL: Final = 1
SUB_HEADING_LEVEL: Final = 2
DEFAULT_FONT_SIZE: Final = 11.0


def _text_blocks(document: Any) -> list[tuple[int, dict[str, Any]]]:
    found = []
    for number, page in enumerate(document, start=1):
        for block in page.get_text("dict", sort=True)["blocks"]:
            if block["type"] == TEXT_BLOCK:  # image blocks have no lines
                found.append((number, block))
    return found


def _block_text(block: dict[str, Any]) -> str:
    return "\n".join(
        "".join(span["text"] for span in line["spans"]).strip() for line in block["lines"]
    ).strip()


def _max_size(block: dict[str, Any]) -> float:
    return max((span["size"] for line in block["lines"] for span in line["spans"]), default=DEFAULT_FONT_SIZE)


class PdfParser:
    """Fallback parser (degraded): text layer only, headings guessed from font size, no tables, no OCR."""

    def parse(self, path: Path) -> ParsedDocument:
        # A failed MuPDF filename open can retain its Windows handle through the exception traceback.
        with pymupdf.open(stream=path.read_bytes(), filetype="pdf") as document:  # type: ignore[no-untyped-call]  # pymupdf ships partial types
            if document.needs_pass:
                raise ValueError("encrypted PDF")  # ParseService maps it to INGEST_PARSE_FAILED
            blocks = _text_blocks(document)
            page_count = document.page_count
        sizes = [span["size"] for _, b in blocks for line in b["lines"] for span in line["spans"]]
        body_size = statistics.median(sizes) if sizes else DEFAULT_FONT_SIZE
        collector = BlockCollector()
        for page, block in blocks:
            text = _block_text(block)
            ratio = _max_size(block) / body_size
            is_heading = ratio >= HEADING_SIZE_RATIO and len(text.split()) <= HEADING_MAX_WORDS
            level = (
                (TOP_HEADING_LEVEL if ratio >= TOP_HEADING_SIZE_RATIO else SUB_HEADING_LEVEL)
                if is_heading
                else None
            )
            block_type = BlockType.HEADING if is_heading else BlockType.TEXT
            collector.add(block_type, text, pdf_bbox(page, tuple(block["bbox"])), level=level)
        return ParsedDocument(collector.blocks(), ParseProvider.LOCAL, page_count=page_count)
