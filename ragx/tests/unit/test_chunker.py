import uuid

from thomas_ragx.ingest.chunker import Chunker, chunk_id_for, detect_lang, point_id_for
from thomas_ragx.ingest.outline import build_outline
from thomas_ragx.parsing.model import (
    BlockCollector,
    BlockType,
    LocatorKind,
    TableData,
    pdf_bbox,
    sheet_range,
    table_text,
    text_lines,
)

FILE_ID = uuid.UUID("00000000-0000-0000-0000-0000000000bb")


def _words(count: int, word: str = "chữ") -> str:
    return " ".join([word] * count)


def _chunk(collector: BlockCollector, chunker: Chunker | None = None) -> list:
    blocks = collector.blocks()
    outline = build_outline(FILE_ID, 1, "tai-lieu.pdf", blocks)
    return (chunker or Chunker(350, 700, 50)).chunk(FILE_ID, 1, "tai-lieu.pdf", blocks, outline)


def test_chunker_joins_blocks_until_the_target_and_records_offsets() -> None:
    collector = BlockCollector()
    for page in range(1, 6):
        collector.add(BlockType.TEXT, _words(100, f"p{page}"), pdf_bbox(page, (0, 0, 1, 1)))
    chunks = _chunk(collector)
    assert [len(c.blocks) for c in chunks] == [4, 1]  # 400 words ≥ 350 closes the first chunk
    first = chunks[0]
    assert (first.page_start, first.page_end) == (1, 4)
    expected_start = 0
    for source, block in zip(collector.blocks()[:4], first.blocks, strict=True):
        assert block.block_id == source.block_id
        assert block.locator == source.locator
        assert block.char_start == expected_start
        assert block.char_end == expected_start + len(source.text)
        assert first.text[block.char_start : block.char_end] == source.text
        expected_start = block.char_end + len("\n\n")
    assert first.blocks[-1].char_end == len(first.text)
    assert first.embed_text.startswith("Tài liệu: tai-lieu.pdf\nTrang: 1\u20134\n\n")


def test_chunker_never_crosses_a_section_boundary() -> None:
    collector = BlockCollector()
    collector.add(BlockType.HEADING, "Điều 1", text_lines(1, 1), level=1)
    collector.add(BlockType.TEXT, _words(20), text_lines(2, 2))
    collector.add(BlockType.HEADING, "Điều 2", text_lines(3, 3), level=1)
    collector.add(BlockType.TEXT, _words(20), text_lines(4, 4))
    chunks = _chunk(collector)
    assert [c.section for c in chunks] == [("Điều 1",), ("Điều 2",)]
    assert chunks[0].node_id != chunks[1].node_id
    assert "Mục: Điều 1" in chunks[0].embed_text


def test_chunker_keeps_a_table_whole_in_its_own_chunk() -> None:
    collector = BlockCollector()
    collector.add(BlockType.TEXT, _words(30), text_lines(1, 1))
    collector.add(
        BlockType.TABLE, "", text_lines(2, 5), table=TableData(("A", "B"), (("1", "2"), ("3", "4")))
    )
    collector.add(BlockType.TEXT, _words(30), text_lines(6, 6))
    chunks = _chunk(collector)
    assert [[b.block_id for b in c.blocks] for c in chunks] == [["b-0001"], ["b-0002"], ["b-0003"]]


def test_chunker_splits_big_tables_by_rows_and_narrows_sheet_ranges() -> None:
    rows = tuple((f"Dòng {n}", _words(60)) for n in range(2, 32))  # 30 rows x ~62 words
    table = TableData(("Tên", "Mô tả"), rows, source_rows=tuple(range(2, 32)), columns=("A", "B"))
    collector = BlockCollector()
    collector.add(BlockType.TABLE, "", sheet_range("Sheet1", "A1:B31"), table=table)
    chunks = _chunk(collector, Chunker(350, 700, 50))
    assert len(chunks) > 1
    assert all(c.text.startswith("| Tên | Mô tả |") for c in chunks)  # the header repeats
    ranges = [c.blocks[0].locator.cell_range for c in chunks]
    assert ranges == ["A2:B11", "A12:B21", "A22:B31"]
    seen_rows = []
    for chunk, start in zip(chunks, (0, 10, 20), strict=True):
        group_rows = rows[start : start + 10]
        assert chunk.text == table_text(TableData(table.header, group_rows))
        assert len(chunk.blocks) == 1
        block = chunk.blocks[0]
        assert block.block_id == "b-0001"
        assert block.locator == sheet_range("Sheet1", f"A{start + 2}:B{start + 11}")
        assert (block.char_start, block.char_end) == (0, len(chunk.text))
        assert chunk.text[block.char_start : block.char_end] == chunk.text
        seen_rows.extend(group_rows)
    assert tuple(seen_rows) == rows
    assert all(len(c.text.split()) <= 700 + 20 for c in chunks)  # pipes and header add a little


def test_chunker_windows_an_oversized_block_with_overlap() -> None:
    collector = BlockCollector()
    collector.add(BlockType.TEXT, " ".join(f"w{n}" for n in range(1500)), pdf_bbox(1, None))
    chunks = _chunk(collector, Chunker(350, 700, 50))
    assert [len(c.text.split()) for c in chunks] == [700, 700, 200]
    assert chunks[1].text.split()[0] == "w650"  # 50 words of overlap
    assert {c.blocks[0].block_id for c in chunks} == {"b-0001"}
    assert {c.blocks[0].locator.kind for c in chunks} == {LocatorKind.PDF_BBOX}


def test_chunk_ids_are_deterministic_and_unique() -> None:
    collector = BlockCollector()
    for _ in range(3):
        collector.add(BlockType.TEXT, _words(400), text_lines(1, 1))
    ids = [c.chunk_id for c in _chunk(collector)]
    assert ids == [c.chunk_id for c in _chunk(collector)] and len(set(ids)) == len(ids)
    assert ids[0] == chunk_id_for(FILE_ID, 1, 0) and len(ids[0]) == 32
    assert point_id_for(ids[0]) == point_id_for(ids[0])


def test_detect_lang() -> None:
    assert detect_lang("Hợp đồng có hiệu lực") == "vi"
    assert detect_lang("The contract is valid") == "en"


def test_chunker_preserves_an_oversized_header_only_table() -> None:
    collector = BlockCollector()
    locator = sheet_range("Sheet1", "A1:B1")
    table = TableData((_words(701, "header"), "B"), (), source_rows=(), columns=("A", "B"))
    collector.add(BlockType.TABLE, "", locator, table=table)
    source = collector.blocks()[0]
    assert len(source.text.split()) > 700
    chunks = _chunk(collector)
    assert len(chunks) == 1
    chunk = chunks[0]
    assert chunk.text == source.text
    assert len(chunk.blocks) == 1
    block = chunk.blocks[0]
    assert block.block_id == source.block_id
    assert block.locator == locator
    assert (block.char_start, block.char_end) == (0, len(source.text))
    assert chunk.text[block.char_start : block.char_end] == source.text
