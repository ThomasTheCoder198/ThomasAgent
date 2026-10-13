import hashlib
import re
import uuid
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Final
from uuid import UUID

from thomas_ragx.ingest.outline import Outline
from thomas_ragx.parsing.model import (
    Block,
    BlockType,
    Locator,
    LocatorKind,
    TableData,
    sheet_range,
    table_text,
)

# Fixed forever (ruling B3): point ids must survive processes and re-ingests.
CHUNK_NAMESPACE: Final[UUID] = UUID("2a7c4e9b-1f3d-5a8e-9c0b-4d6f8a1e2b3c")
CHUNK_ID_LENGTH: Final = 32
BLOCK_SEPARATOR: Final = "\n\n"
SECTION_SEPARATOR: Final = " > "
PAGE_RANGE_SEPARATOR: Final = "\u2013"
HEADER_DOCUMENT: Final = "Tài liệu: "
HEADER_SECTION: Final = "Mục: "
HEADER_PAGES: Final = "Trang: "
VIETNAMESE_LETTERS: Final = re.compile(
    r"[ăâđêôơưáàảãạấầẩẫậắằẳẵặéèẻẽẹếềểễệíìỉĩịóòỏõọốồổỗộớờởỡợúùủũụứừửữựýỳỷỹỵ]", re.IGNORECASE
)
LANG_VI: Final = "vi"
LANG_EN: Final = "en"


def chunk_id_for(file_id: UUID, version: int, index: int) -> str:
    return hashlib.sha256(f"{file_id}:{version}:{index}".encode()).hexdigest()[:CHUNK_ID_LENGTH]


def point_id_for(chunk_id: str) -> UUID:
    return uuid.uuid5(CHUNK_NAMESPACE, chunk_id)


def detect_lang(text: str) -> str:
    return LANG_VI if VIETNAMESE_LETTERS.search(text) else LANG_EN


def word_count(text: str) -> int:
    return len(text.split())


@dataclass(frozen=True)
class ChunkBlock:
    block_id: str
    locator: Locator
    char_start: int
    char_end: int


@dataclass(frozen=True)
class Chunk:
    chunk_id: str
    index: int
    node_id: UUID
    section: tuple[str, ...]
    text: str
    embed_text: str
    blocks: tuple[ChunkBlock, ...]
    page_start: int | None
    page_end: int | None
    lang: str


@dataclass
class _Draft:
    node_id: UUID
    parts: list[tuple[str, str, Locator]] = field(default_factory=list)  # (block_id, text, locator)

    def words(self) -> int:
        return sum(word_count(text) for _, text, _ in self.parts)


class Chunker:
    """Block-based chunks (addendum §5.1, ruling B9). Pure: same input, same chunks and ids."""

    def __init__(self, target_words: int, max_words: int, overlap_words: int) -> None:
        self._target = target_words
        self._max = max_words
        self._overlap = overlap_words

    def chunk(
        self, file_id: UUID, version: int, file_name: str, blocks: Sequence[Block], outline: Outline
    ) -> list[Chunk]:
        drafts: list[_Draft] = []
        current: _Draft | None = None
        for block in blocks:
            node_id = outline.block_nodes[block.block_id]
            if current is not None and (current.node_id != node_id or block.block_type is BlockType.TABLE):
                drafts.append(current)
                current = None
            if block.block_type is BlockType.TABLE and block.table is not None:
                drafts += [_Draft(node_id, [part]) for part in self._table_parts(block, block.table)]
                continue
            if word_count(block.text) > self._max:
                if current is not None:
                    drafts.append(current)
                    current = None
                drafts += [_Draft(node_id, [part]) for part in self._windows(block)]
                continue
            if current is None:
                current = _Draft(node_id)
            elif current.words() + word_count(block.text) > self._max:
                drafts.append(current)
                current = _Draft(node_id)
            current.parts.append((block.block_id, block.text, block.locator))
            if current.words() >= self._target:
                drafts.append(current)
                current = None
        if current is not None:
            drafts.append(current)
        return [self._build(file_id, version, file_name, index, d, outline) for index, d in enumerate(drafts)]

    def _windows(self, block: Block) -> list[tuple[str, str, Locator]]:
        words = block.text.split()
        step = self._max - self._overlap
        starts = range(0, max(len(words) - self._overlap, 1), step)
        return [
            (block.block_id, " ".join(words[start : start + self._max]), block.locator) for start in starts
        ]

    def _table_parts(self, block: Block, table: TableData) -> list[tuple[str, str, Locator]]:
        # With no body rows there is no row boundary to split at; preserve the header intact.
        if not table.rows or word_count(block.text) <= self._max:
            return [(block.block_id, block.text, block.locator)]
        parts: list[tuple[str, str, Locator]] = []
        start = 0
        while start < len(table.rows):
            end = start + 1
            while (
                end < len(table.rows)
                and word_count(table_text(TableData(table.header, table.rows[start : end + 1]))) <= self._max
            ):
                end += 1
            group = TableData(table.header, table.rows[start:end])
            parts.append(
                (block.block_id, table_text(group), self._row_locator(block.locator, table, start, end))
            )
            start = end
        return parts

    @staticmethod
    def _row_locator(locator: Locator, table: TableData, start: int, end: int) -> Locator:
        if locator.kind is not LocatorKind.SHEET_RANGE or table.source_rows is None or table.columns is None:
            return locator
        first, last = table.columns
        rows = table.source_rows[start:end]
        return sheet_range(locator.sheet or "", f"{first}{rows[0]}:{last}{rows[-1]}")

    def _build(
        self, file_id: UUID, version: int, file_name: str, index: int, draft: _Draft, outline: Outline
    ) -> Chunk:
        pieces: list[ChunkBlock] = []
        offset = 0
        for block_id, text, locator in draft.parts:
            pieces.append(ChunkBlock(block_id, locator, offset, offset + len(text)))
            offset += len(text) + len(BLOCK_SEPARATOR)
        text = BLOCK_SEPARATOR.join(text for _, text, _ in draft.parts)
        pages = [p.locator.page for p in pieces if p.locator.page is not None]
        section = outline.paths.get(draft.node_id, ())
        chunk_id = chunk_id_for(file_id, version, index)
        return Chunk(
            chunk_id=chunk_id,
            index=index,
            node_id=draft.node_id,
            section=section,
            text=text,
            embed_text=_embed_text(
                file_name, section, min(pages) if pages else None, max(pages) if pages else None, text
            ),
            blocks=tuple(pieces),
            page_start=min(pages) if pages else None,
            page_end=max(pages) if pages else None,
            lang=detect_lang(text),
        )


def _embed_text(
    file_name: str, section: tuple[str, ...], first: int | None, last: int | None, text: str
) -> str:
    lines = [HEADER_DOCUMENT + file_name]
    if section:
        lines.append(HEADER_SECTION + SECTION_SEPARATOR.join(section))
    if first is not None:
        pages = str(first) if first == last else f"{first}{PAGE_RANGE_SEPARATOR}{last}"
        lines.append(HEADER_PAGES + pages)
    return "\n".join(lines) + BLOCK_SEPARATOR + text
