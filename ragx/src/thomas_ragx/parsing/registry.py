from collections.abc import Mapping
from pathlib import Path
from typing import Final, Protocol

from thomas_ragx.parsing.html_files import HtmlParser
from thomas_ragx.parsing.model import ParsedDocument
from thomas_ragx.parsing.office import DocxParser, PptxParser
from thomas_ragx.parsing.pdf_local import PdfParser
from thomas_ragx.parsing.sheets import CsvParser, XlsxParser
from thomas_ragx.parsing.text_files import MarkdownParser, TextParser

# Images need OCR; without Datalab there is nothing to parse (ruling A21).
IMAGE_EXTENSIONS: Final = frozenset({".png", ".jpg", ".jpeg"})


class Parser(Protocol):
    def parse(self, path: Path) -> ParsedDocument: ...


class ParserRegistry:
    """Plugin registry by extension (main spec §4.1): a new format adds an entry, not a pipeline change."""

    def __init__(self, parsers: Mapping[str, Parser]) -> None:
        self._parsers = dict(parsers)

    def for_extension(self, extension: str) -> Parser | None:
        return self._parsers.get(extension.lower())


def local_registry() -> ParserRegistry:
    html = HtmlParser()
    return ParserRegistry(
        {
            ".md": MarkdownParser(),
            ".txt": TextParser(),
            ".csv": CsvParser(),
            ".xlsx": XlsxParser(),
            ".html": html,
            ".htm": html,
            ".docx": DocxParser(),
            ".pptx": PptxParser(),
            ".pdf": PdfParser(),
        }
    )
