import re
from collections.abc import Iterator, Mapping
from typing import Any, Final

from bs4 import BeautifulSoup, Tag

from thomas_ragx.parsing.locators import DATALAB_LOCATOR_KINDS
from thomas_ragx.parsing.model import (
    BBox,
    BlockCollector,
    BlockType,
    Locator,
    ParsedDocument,
    ParseProvider,
    TableData,
)
from thomas_ragx.parsing.office import BULLET_MARKER, LIST_INDENT, NUMBER_MARKER_SUFFIX
from thomas_ragx.parsing.page_sizes import PageSize, SourcePageSizes

# Field names of Datalab's JSON (measured by spike S9: docs/superpowers/spikes/S9-datalab.md).
KEY_CHILDREN: Final = "children"
KEY_TYPE: Final = "block_type"
KEY_HTML: Final = "html"
KEY_BBOX: Final = "bbox"
TYPE_PAGE: Final = "Page"
HTML_PARSER: Final = "html.parser"
HEADING_TAG: Final = re.compile(r"<h([1-6])\b", re.IGNORECASE)
DEFAULT_HEADING_LEVEL: Final = 2
BBOX_LENGTH: Final = 4
POINT_DECIMALS: Final = 2
# A hostile or broken span must not allocate an unbounded grid.
MAX_TABLE_COLUMNS: Final = 1000
MAX_TABLE_CELLS: Final = 100_000
TABLE_TAG: Final = "table"
ROW_TAG: Final = "tr"
CELL_TAGS: Final = ("th", "td")
LIST_TAGS: Final = ("ul", "ol")
ORDERED_LIST_TAG: Final = "ol"
ITEM_TAG: Final = "li"
BREAK_TAG: Final = "br"
NEWLINE: Final = "\n"
ROWSPAN_ATTR: Final = "rowspan"
COLSPAN_ATTR: Final = "colspan"

SKIPPED_TYPES: Final = frozenset({"PageHeader", "PageFooter", "Line", "Span", "Reference"})
# Groups wrap a table or figure with its caption; their children are the blocks to keep.
DESCENDED_GROUPS: Final = frozenset({"TableGroup", "FigureGroup", "PictureGroup"})
BLOCK_TYPES: Final[Mapping[str, BlockType]] = {
    "SectionHeader": BlockType.HEADING,
    "Text": BlockType.TEXT,
    "TextInlineMath": BlockType.TEXT,
    "Handwriting": BlockType.TEXT,
    "Footnote": BlockType.TEXT,
    "Form": BlockType.TEXT,
    "ComplexRegion": BlockType.TEXT,
    "TableOfContents": BlockType.TEXT,
    "ListGroup": BlockType.LIST,
    "ListItem": BlockType.LIST,
    "Table": BlockType.TABLE,
    "Figure": BlockType.FIGURE,
    "Picture": BlockType.FIGURE,
    "Caption": BlockType.CAPTION,
    "Code": BlockType.CODE,
    "Equation": BlockType.FORMULA,
}

# Tags after which words must not run together once the markup is gone.
SEPARATING_TAGS: Final = (
    "p", "div", "li", "ul", "ol", "br", "tr", "td", "th", "blockquote", "pre",
    "h1", "h2", "h3", "h4", "h5", "h6",
)  # fmt: skip
MATH_TAG: Final = "math"
LATEX_COMMAND: Final = re.compile(r"\\[A-Za-z]+")
# Datalab returns symbols as <math>LaTeX</math> (ruling S9-6); unknown commands stay as the raw LaTeX.
LATEX_SYMBOLS: Final[Mapping[str, str]] = {
    r"\leq": "≤",
    r"\le": "≤",
    r"\geq": "≥",
    r"\ge": "≥",
    r"\neq": "≠",
    r"\ne": "≠",
    r"\approx": "≈",
    r"\pm": "±",
    r"\times": "\N{MULTIPLICATION SIGN}",
    r"\cdot": "·",
    r"\rightarrow": "→",
    r"\to": "→",
    r"\leftarrow": "←",
    r"\degree": "°",
}


def _latex_symbol(match: re.Match[str]) -> str:
    return LATEX_SYMBOLS.get(match.group(), match.group())


def _soup_text(root: BeautifulSoup | Tag) -> str:
    for math in root.find_all(MATH_TAG):
        math.replace_with(LATEX_COMMAND.sub(_latex_symbol, math.get_text()))
    for tag in root.find_all(SEPARATING_TAGS):
        tag.insert_after(" ")
    return " ".join(root.get_text("").split())


def text_from_html(html: str) -> str:
    return _soup_text(BeautifulSoup(html, HTML_PARSER))


def _span(cell: Tag, attribute: str, limit: int) -> int:
    try:
        value = int(str(cell.get(attribute, "1")))
    except ValueError:
        return 1
    return min(max(value, 1), max(limit, 1))


def _outer_rows(soup: BeautifulSoup) -> list[Tag]:
    """Rows of the first table only: a nested table folds into its cell's text instead of adding rows."""
    table = soup.find(TABLE_TAG)
    return [row for row in soup.find_all(ROW_TAG) if row.find_parent(TABLE_TAG) is table]


def table_from_html(html: str) -> TableData | None:
    """A rectangular grid: a spanning cell repeats its text in every slot it covers (ruling S9-5).

    Width and cell count are bounded, so a hostile span cannot allocate an unbounded grid.
    """
    rows = _outer_rows(BeautifulSoup(html, HTML_PARSER))
    if not rows:
        return None
    grid: dict[tuple[int, int], str] = {}
    width = 0
    for row_index, row in enumerate(rows):
        column = 0
        for cell in row.find_all(CELL_TAGS, recursive=False):
            while (row_index, column) in grid:
                column += 1
            if column >= MAX_TABLE_COLUMNS or len(grid) >= MAX_TABLE_CELLS:
                break
            text = _soup_text(cell)
            rowspan = _span(cell, ROWSPAN_ATTR, len(rows) - row_index)
            colspan = _span(cell, COLSPAN_ATTR, MAX_TABLE_COLUMNS - column)
            rowspan = min(rowspan, max(1, (MAX_TABLE_CELLS - len(grid)) // colspan))
            for down in range(rowspan):
                for across in range(colspan):
                    grid[(row_index + down, column + across)] = text
            column += colspan
            width = max(width, column)
    height = max(1, min(len(rows), MAX_TABLE_CELLS // max(width, 1)))
    cells = [tuple(grid.get((r, c), "") for c in range(width)) for r in range(height)]
    return TableData(header=cells[0], rows=tuple(cells[1:]))


def _own_text(item: Tag) -> str:
    nested = [inner for inner in item.find_all(LIST_TAGS) if inner.find_parent(ITEM_TAG) is item]
    for inner in nested:
        inner.extract()
    return _soup_text(item)


def _list_lines(list_tag: Tag, depth: int) -> list[str]:
    ordered = list_tag.name == ORDERED_LIST_TAG
    lines: list[str] = []
    for number, item in enumerate(list_tag.find_all(ITEM_TAG, recursive=False), start=1):
        nested = [inner for inner in item.find_all(LIST_TAGS) if inner.find_parent(ITEM_TAG) is item]
        marker = f"{number}{NUMBER_MARKER_SUFFIX}" if ordered else BULLET_MARKER
        lines.append(LIST_INDENT * depth + marker + _own_text(item))
        for inner in nested:
            lines.extend(_list_lines(inner, depth + 1))
    return lines


def list_text_from_html(html: str) -> str:
    """One line per item, in the shape the local DOCX and PPTX parsers give a list block."""
    soup = BeautifulSoup(html, HTML_PARSER)
    lists = [tag for tag in soup.find_all(LIST_TAGS) if tag.find_parent(LIST_TAGS) is None]
    if lists:
        return NEWLINE.join(line for tag in lists for line in _list_lines(tag, 0))
    items = soup.find_all(ITEM_TAG)
    if items:
        return NEWLINE.join(BULLET_MARKER + _own_text(item) for item in items)
    return _soup_text(soup)


def code_text_from_html(html: str) -> str:
    """Code keeps its line breaks and indentation; only the markup goes."""
    soup = BeautifulSoup(html, HTML_PARSER)
    for line_break in soup.find_all(BREAK_TAG):
        line_break.replace_with(NEWLINE)
    return soup.get_text("").strip(NEWLINE)


def _block_text(block_type: BlockType, html: str) -> str:
    if block_type is BlockType.LIST:
        return list_text_from_html(html)
    if block_type is BlockType.CODE:
        return code_text_from_html(html)
    return text_from_html(html)


def _bbox(node: Mapping[str, Any]) -> BBox | None:
    value = node.get(KEY_BBOX)
    if not isinstance(value, list) or len(value) != BBOX_LENGTH:
        return None
    return (float(value[0]), float(value[1]), float(value[2]), float(value[3]))


def _page_extent(page: Mapping[str, Any]) -> PageSize | None:
    """Size in pixels of Datalab's rendering of the page: the Page node's own box."""
    box = _bbox(page)
    if box is None:
        return None
    width, height = box[2] - box[0], box[3] - box[1]
    return (width, height) if width > 0 and height > 0 else None


def _points(box: BBox, extent: PageSize, source: PageSize) -> BBox:
    scale_x, scale_y = source[0] / extent[0], source[1] / extent[1]  # per axis: W and H round separately
    return (
        round(box[0] * scale_x, POINT_DECIMALS),
        round(box[1] * scale_y, POINT_DECIMALS),
        round(box[2] * scale_x, POINT_DECIMALS),
        round(box[3] * scale_y, POINT_DECIMALS),
    )


def _leaves(node: Mapping[str, Any]) -> Iterator[Mapping[str, Any]]:
    block_type = node.get(KEY_TYPE)
    if block_type in SKIPPED_TYPES:
        return
    if block_type in DESCENDED_GROUPS:
        for child in node.get(KEY_CHILDREN) or []:
            yield from _leaves(child)
        return
    if block_type in BLOCK_TYPES:
        yield node  # Line/Span/ListItem children are not separate blocks (citation stays block-level)


def to_parsed_document(
    tree: Mapping[str, Any],
    extension: str,
    *,
    page_count: int | None,
    quality: float | None,
    page_sizes: SourcePageSizes | None = None,
) -> ParsedDocument:
    """Without a source page size a block cites its page only, never raw pixels (ruling S9-3)."""
    kind = DATALAB_LOCATOR_KINDS[extension]
    collector = BlockCollector()
    pages = [child for child in tree.get(KEY_CHILDREN) or [] if child.get(KEY_TYPE) == TYPE_PAGE]
    for page_number, page in enumerate(pages, start=1):  # numbered by order, whatever the id base is
        extent = _page_extent(page)
        source = page_sizes.size(page_number) if page_sizes is not None else None
        for child in page.get(KEY_CHILDREN) or []:
            for leaf in _leaves(child):
                block_type = BLOCK_TYPES[str(leaf[KEY_TYPE])]
                html = str(leaf.get(KEY_HTML) or "")
                pixels = _bbox(leaf)
                box = _points(pixels, extent, source) if pixels and extent and source else None
                locator = Locator(kind, page=page_number, bbox=box)
                if block_type is BlockType.TABLE:
                    table = table_from_html(html)
                    if table is not None:
                        collector.add(block_type, "", locator, table=table)
                    continue
                match = HEADING_TAG.search(html)
                level = int(match.group(1)) if match else DEFAULT_HEADING_LEVEL
                collector.add(
                    block_type,
                    _block_text(block_type, html),
                    locator,
                    level=level if block_type is BlockType.HEADING else None,
                )
    return ParsedDocument(collector.blocks(), ParseProvider.DATALAB, page_count=page_count, quality=quality)
