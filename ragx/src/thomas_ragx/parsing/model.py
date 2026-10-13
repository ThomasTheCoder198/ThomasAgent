from dataclasses import dataclass, replace
from enum import StrEnum
from typing import Final

BLOCK_ID_PREFIX: Final = "b-"
BLOCK_ID_DIGITS: Final = 4
TABLE_CELL_SEPARATOR: Final = " | "
TABLE_RULE_CELL: Final = "---"
# Postgres text and jsonb reject NUL; PDF text layers and mis-decoded UTF-16 can carry it.
NUL: Final = "\x00"
PIPE: Final = "|"
ESCAPED_PIPE: Final = "\\|"

type BBox = tuple[float, float, float, float]


class LocatorKind(StrEnum):
    PDF_BBOX = "pdf_bbox"
    SHEET_RANGE = "sheet_range"
    TEXT_LINES = "text_lines"
    DOCX_PARAGRAPH = "docx_paragraph"


class BlockType(StrEnum):
    HEADING = "heading"
    TEXT = "text"
    LIST = "list"
    TABLE = "table"
    FIGURE = "figure"
    CODE = "code"
    CAPTION = "caption"
    FORMULA = "formula"


class ParseProvider(StrEnum):
    DATALAB = "datalab"
    LOCAL = "local"


@dataclass(frozen=True)
class Locator:
    """Where a block lives in its source (glossary: Locator). One kind per file type."""

    kind: LocatorKind
    page: int | None = None
    bbox: BBox | None = None
    sheet: str | None = None
    cell_range: str | None = None
    line_start: int | None = None
    line_end: int | None = None
    paragraph: int | None = None

    def to_json(self) -> dict[str, object]:
        fields: dict[str, object | None] = {
            "page": self.page,
            "bbox": list(self.bbox) if self.bbox is not None else None,
            "sheet": self.sheet,
            "range": self.cell_range,
            "lineStart": self.line_start,
            "lineEnd": self.line_end,
            "paragraph": self.paragraph,
        }
        return {"kind": self.kind.value, **{key: value for key, value in fields.items() if value is not None}}


def pdf_bbox(page: int, bbox: BBox | None) -> Locator:
    return Locator(LocatorKind.PDF_BBOX, page=page, bbox=bbox)


def sheet_range(sheet: str, cell_range: str) -> Locator:
    return Locator(LocatorKind.SHEET_RANGE, sheet=sheet, cell_range=cell_range)


def text_lines(line_start: int, line_end: int) -> Locator:
    return Locator(LocatorKind.TEXT_LINES, line_start=line_start, line_end=line_end)


def docx_paragraph(paragraph: int) -> Locator:
    return Locator(LocatorKind.DOCX_PARAGRAPH, paragraph=paragraph)


@dataclass(frozen=True)
class TableData:
    """A table kept whole as a block; source_rows/columns let the chunker cite a row group (M1.3b)."""

    header: tuple[str, ...]
    rows: tuple[tuple[str, ...], ...]
    source_rows: tuple[int, ...] | None = None
    columns: tuple[str, str] | None = None


def _cell(value: str) -> str:
    return value.replace(PIPE, ESCAPED_PIPE).replace("\n", " ")


def table_text(table: TableData) -> str:
    def row(cells: tuple[str, ...]) -> str:
        return f"{PIPE} {TABLE_CELL_SEPARATOR.join(_cell(c) for c in cells)} {PIPE}"

    rule = row(tuple(TABLE_RULE_CELL for _ in table.header))
    return "\n".join([row(table.header), rule, *(row(r) for r in table.rows)])


def _without_nul_table(table: TableData) -> TableData:
    def clean(cells: tuple[str, ...]) -> tuple[str, ...]:
        return tuple(cell.replace(NUL, "") for cell in cells)

    return replace(table, header=clean(table.header), rows=tuple(clean(row) for row in table.rows))


@dataclass(frozen=True)
class Block:
    block_id: str
    block_type: BlockType
    text: str
    locator: Locator
    level: int | None = None
    table: TableData | None = None


@dataclass(frozen=True)
class ParsedDocument:
    blocks: tuple[Block, ...]
    provider: ParseProvider
    page_count: int | None = None
    quality: float | None = None
    degraded: bool = False


def block_id(position: int) -> str:
    return f"{BLOCK_ID_PREFIX}{position + 1:0{BLOCK_ID_DIGITS}d}"


class BlockCollector:
    """Numbers blocks in document order; ids are stable for one parse of one file version."""

    def __init__(self) -> None:
        self._blocks: list[Block] = []

    def add(
        self,
        block_type: BlockType,
        text: str,
        locator: Locator,
        *,
        level: int | None = None,
        table: TableData | None = None,
    ) -> None:
        clean_table = None if table is None else _without_nul_table(table)
        cleaned = text.replace(NUL, "").strip() if clean_table is None else table_text(clean_table)
        if not cleaned:
            return
        self._blocks.append(
            Block(block_id(len(self._blocks)), block_type, cleaned, locator, level, clean_table)
        )

    def blocks(self) -> tuple[Block, ...]:
        return tuple(self._blocks)
