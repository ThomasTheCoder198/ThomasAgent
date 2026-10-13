from collections.abc import Iterator
from enum import StrEnum
from pathlib import Path
from typing import Final

from bs4 import BeautifulSoup, Tag
from bs4.element import NavigableString, PageElement, PreformattedString

from thomas_ragx.parsing.model import (
    Block,
    BlockCollector,
    BlockType,
    ParsedDocument,
    ParseProvider,
    TableData,
    text_lines,
)
from thomas_ragx.parsing.text_files import read_text

# html.parser records sourceline for every tag; lxml's HTML parser does not.
HTML_PARSER: Final = "html.parser"
DROPPED_TAGS: Final = ("script", "style", "noscript", "template", "head", "title")
HEADING_TAGS: Final = {"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}
LIST_TAGS: Final = frozenset({"ul", "ol"})
BLOCK_TAGS: Final = frozenset({*HEADING_TAGS, "p", "pre", "table", "blockquote", *LIST_TAGS})
# Boundaries for the fallback below: text on either side of one of these tags is a separate block. Any other
# tag (span, a, b, ...) is inline and its text joins the surrounding run.
CONTAINER_TAGS: Final = frozenset(
    {
        "html",
        "body",
        "div",
        "section",
        "article",
        "main",
        "aside",
        "header",
        "footer",
        "nav",
        "form",
        "figure",
        "figcaption",
        "address",
        "details",
        "summary",
        "dl",
        "dt",
        "dd",
        "li",
        "tr",
        "td",
        "th",
        "center",
    }
)
# A line break or a form control ends a word even though no whitespace follows it in the source.
BREAK_TAG: Final = "br"
SEPARATOR_TAGS: Final = frozenset(
    {BREAK_TAG, "label", "legend", "input", "select", "option", "optgroup", "textarea", "button"}
)
WORD_SEPARATOR: Final = " "
PRE_LINE_BREAK: Final = "\n"
LIST_ITEM_PREFIX: Final = "- "
FIRST_LINE: Final = 1


def _rows(table: Tag) -> TableData | None:
    rows = [
        tuple(cell.get_text(" ", strip=True) for cell in tr.find_all(["th", "td"]))
        for tr in table.find_all("tr")
    ]
    if not any(any(row) for row in rows):
        return None
    return TableData(header=rows[0], rows=tuple(rows[1:]))


def _text_of(element: Tag, break_text: str) -> str:
    """Like get_text, but a separator tag leaves `break_text` behind so adjacent words do not fuse."""
    parts: list[str] = []
    for node in element.descendants:
        if isinstance(node, Tag):
            if node.name in SEPARATOR_TAGS:
                parts.append(break_text if node.name == BREAK_TAG else WORD_SEPARATOR)
        elif isinstance(node, NavigableString) and not isinstance(node, PreformattedString):
            parts.append(str(node))
    return "".join(parts)


class _OnExit(StrEnum):
    NOTHING = "nothing"
    FLUSH = "flush"  # a container ends the current text run
    SEPARATE = "separate"  # a separator tag ends a word


class _HtmlReader:
    """Walks the tree in document order, iteratively: html.parser nests unclosed tags, so a malformed page can
    be thousands of levels deep. The block tags are the main path; text that sits directly in a container
    (a page built from `div`s) would otherwise vanish and end in INGEST_NO_TEXT, so each run of inline text
    between block tags becomes a text block too. A block tag's own text is never read twice."""

    def __init__(self) -> None:
        self._collector = BlockCollector()
        self._parts: list[str] = []
        self._start: int | None = None
        self._end = FIRST_LINE
        self._line = FIRST_LINE  # best known source line of the node being read

    def read(self, soup: Tag) -> tuple[Block, ...]:
        stack: list[tuple[Iterator[PageElement], _OnExit]] = [(iter(soup.children), _OnExit.NOTHING)]
        while stack:
            children, on_exit = stack[-1]
            node = next(children, None)
            if node is None:
                stack.pop()
                self._leave(on_exit)
            elif isinstance(node, Tag):
                entered = self._enter(node)
                if entered is not None:
                    stack.append(entered)
            elif isinstance(node, NavigableString) and not isinstance(node, PreformattedString):
                self._text(str(node))  # PreformattedString: comments, doctype, CDATA
        self._flush()
        return self._collector.blocks()

    def _enter(self, tag: Tag) -> tuple[Iterator[PageElement], _OnExit] | None:
        self._line = tag.sourceline or self._line
        if tag.name in BLOCK_TAGS:
            self._flush()
            self._block(tag)
            self._line += str(tag).count("\n")
            return None
        if tag.name in CONTAINER_TAGS:
            self._flush()
            return iter(tag.children), _OnExit.FLUSH
        if tag.name in SEPARATOR_TAGS:
            self._text(WORD_SEPARATOR)
            return iter(tag.children), _OnExit.SEPARATE
        return iter(tag.children), _OnExit.NOTHING

    def _leave(self, on_exit: _OnExit) -> None:
        if on_exit is _OnExit.FLUSH:
            self._flush()
        elif on_exit is _OnExit.SEPARATE:
            self._text(WORD_SEPARATOR)

    def _text(self, text: str) -> None:
        if text.strip():
            if self._start is None:
                self._start = self._line + text[: len(text) - len(text.lstrip())].count("\n")
            self._end = self._line + text.rstrip().count("\n")
            self._parts.append(text)
        elif self._parts:
            self._parts.append(text)  # the space between two inline tags
        self._line += text.count("\n")

    def _flush(self) -> None:
        text = " ".join("".join(self._parts).split())
        if text and self._start is not None:
            self._collector.add(BlockType.TEXT, text, text_lines(self._start, self._end))
        self._parts = []
        self._start = None

    def _block(self, element: Tag) -> None:
        start = element.sourceline or self._line
        locator = text_lines(start, start + str(element).count("\n"))
        if element.name in HEADING_TAGS:
            heading_text = element.get_text(" ", strip=True)
            self._collector.add(BlockType.HEADING, heading_text, locator, level=HEADING_TAGS[element.name])
        elif element.name == "table":
            table = _rows(element)
            if table is not None:
                self._collector.add(BlockType.TABLE, "", locator, table=table)
        elif element.name in LIST_TAGS:
            items = [LIST_ITEM_PREFIX + li.get_text(" ", strip=True) for li in element.find_all("li")]
            self._collector.add(BlockType.LIST, "\n".join(items), locator)
        elif element.name == "pre":
            self._collector.add(BlockType.CODE, _text_of(element, PRE_LINE_BREAK), locator)
        else:
            self._collector.add(BlockType.TEXT, " ".join(_text_of(element, WORD_SEPARATOR).split()), locator)


class HtmlParser:
    def parse(self, path: Path) -> ParsedDocument:
        soup = BeautifulSoup(read_text(path), HTML_PARSER)
        for dropped in soup.find_all(DROPPED_TAGS):
            dropped.decompose()
        return ParsedDocument(_HtmlReader().read(soup), ParseProvider.LOCAL)
