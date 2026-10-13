import codecs
import unicodedata
from pathlib import Path
from typing import Final

from markdown_it import MarkdownIt
from markdown_it.token import Token

from thomas_ragx.parsing.model import (
    BlockCollector,
    BlockType,
    ParsedDocument,
    ParseProvider,
    TableData,
    text_lines,
)

# utf-8-sig first; cp1258 is the Windows Vietnamese code page; latin-1 never fails.
TEXT_ENCODINGS: Final = ("utf-8-sig", "cp1258", "latin-1")
NORMAL_FORM: Final = "NFC"
# Excel's "Unicode Text" and Notepad write UTF-16 with a byte order mark; as cp1258 it reads as NUL garbage.
# UTF-32 comes first because its little-endian mark begins with the UTF-16 one. The "utf-16" and "utf-32"
# codecs consume the mark themselves.
BOM_ENCODINGS: Final = (
    (codecs.BOM_UTF32_LE, "utf-32"),
    (codecs.BOM_UTF32_BE, "utf-32"),
    (codecs.BOM_UTF16_LE, "utf-16"),
    (codecs.BOM_UTF16_BE, "utf-16"),
)
HEADING_TAG_PREFIX: Final = "h"
TOP_LEVEL: Final = 0
LIST_OPEN: Final = frozenset({"bullet_list_open", "ordered_list_open"})
CODE_TOKENS: Final = frozenset({"fence", "code_block"})
TEXT_OPEN: Final = frozenset({"paragraph_open", "blockquote_open", "html_block"})


def _encodings_for(data: bytes) -> tuple[str, ...]:
    for bom, encoding in BOM_ENCODINGS:
        if data.startswith(bom):
            return (encoding, *TEXT_ENCODINGS)
    return TEXT_ENCODINGS


def read_text(path: Path) -> str:
    data = path.read_bytes()
    for encoding in _encodings_for(data):
        try:
            # NFC so a cp1258 file (base letter + combining mark) matches the precomposed text users type.
            return unicodedata.normalize(NORMAL_FORM, data.decode(encoding))
        except UnicodeDecodeError:
            continue
    raise UnicodeDecodeError("text", data, 0, len(data), "no known encoding")  # unreachable with latin-1


class TextParser:
    """Paragraphs are runs of non-blank lines; the locator is their 1-based line span."""

    def parse(self, path: Path) -> ParsedDocument:
        collector = BlockCollector()
        start: int | None = None
        lines = read_text(path).splitlines()
        for number, line in enumerate([*lines, ""], start=1):
            if line.strip() and start is None:
                start = number
            elif not line.strip() and start is not None:
                collector.add(
                    BlockType.TEXT, "\n".join(lines[start - 1 : number - 1]), text_lines(start, number - 1)
                )
                start = None
        return ParsedDocument(collector.blocks(), ParseProvider.LOCAL)


def _table(tokens: list[Token], start: int) -> TableData:
    rows: list[tuple[str, ...]] = []
    cells: list[str] = []
    for token in tokens[start:]:
        if token.type == "table_close":
            break
        if token.type == "inline":
            cells.append(token.content.strip())
        elif token.type == "tr_close":
            rows.append(tuple(cells))
            cells = []
    return TableData(header=rows[0] if rows else (), rows=tuple(rows[1:]))


class MarkdownParser:
    """Top-level CommonMark blocks with their source line span (markdown-it `map`: 0-based, end exclusive)."""

    def __init__(self) -> None:
        self._markdown = MarkdownIt("commonmark").enable("table")

    def parse(self, path: Path) -> ParsedDocument:
        source = read_text(path)
        lines = source.splitlines()
        tokens = self._markdown.parse(source)
        collector = BlockCollector()
        for index, token in enumerate(tokens):
            if token.level != TOP_LEVEL or token.map is None or token.nesting < 0:
                continue
            first, end = token.map
            locator = text_lines(first + 1, end)
            body = "\n".join(lines[first:end])
            if token.type == "heading_open":
                level = int(token.tag.removeprefix(HEADING_TAG_PREFIX))
                collector.add(BlockType.HEADING, tokens[index + 1].content, locator, level=level)
            elif token.type == "table_open":
                collector.add(BlockType.TABLE, "", locator, table=_table(tokens, index))
            elif token.type in LIST_OPEN:
                collector.add(BlockType.LIST, body, locator)
            elif token.type in CODE_TOKENS:
                collector.add(BlockType.CODE, token.content, locator)
            elif token.type in TEXT_OPEN:
                collector.add(BlockType.TEXT, body, locator)
        return ParsedDocument(collector.blocks(), ParseProvider.LOCAL)
