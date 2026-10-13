import codecs
from pathlib import Path

import pytest

from thomas_ragx.parsing.model import BlockType, LocatorKind
from thomas_ragx.parsing.sheets import CsvParser
from thomas_ragx.parsing.text_files import MarkdownParser, TextParser


def test_text_parser_splits_paragraphs_with_one_based_lines(tmp_path: Path) -> None:
    path = tmp_path / "a.txt"
    path.write_text("Dòng một\ndòng hai\n\n\nĐoạn hai\n", encoding="utf-8")
    blocks = TextParser().parse(path).blocks
    assert [(b.text, b.locator.line_start, b.locator.line_end) for b in blocks] == [
        ("Dòng một\ndòng hai", 1, 2),
        ("Đoạn hai", 5, 5),
    ]
    assert {b.locator.kind for b in blocks} == {LocatorKind.TEXT_LINES}


def test_text_parser_reads_windows_vietnamese_encoding(tmp_path: Path) -> None:
    path = tmp_path / "cp.txt"
    # cp1258 has no precomposed "ợ"/"ồ": Windows stores the base letter plus a combining tone mark.
    path.write_bytes("Hợp đồng".encode("cp1258"))
    assert TextParser().parse(path).blocks[0].text == "Hợp đồng"


def test_markdown_parser_keeps_headings_lists_tables_and_code(tmp_path: Path) -> None:
    path = tmp_path / "a.md"
    path.write_text(
        "# Chính sách\n\nĐoạn mở đầu.\n\n## Bảng giá\n\n| Gói | Giá |\n| --- | --- |\n| A | 10 |\n\n"
        "- một\n- hai\n\n```\ncode\n```\n",
        encoding="utf-8",
    )
    blocks = MarkdownParser().parse(path).blocks
    assert [(b.block_type, b.level) for b in blocks] == [
        (BlockType.HEADING, 1),
        (BlockType.TEXT, None),
        (BlockType.HEADING, 2),
        (BlockType.TABLE, None),
        (BlockType.LIST, None),
        (BlockType.CODE, None),
    ]
    table = blocks[3].table
    assert table is not None and table.header == ("Gói", "Giá") and table.rows == (("A", "10"),)
    assert (blocks[0].locator.line_start, blocks[3].locator.line_start, blocks[3].locator.line_end) == (
        1,
        7,
        9,
    )


@pytest.mark.parametrize("encoding", ["utf-16", "utf-16-be", "utf-32", "utf-32-be"])
def test_text_parser_reads_utf16_and_utf32_files_with_a_byte_order_mark(
    tmp_path: Path, encoding: str
) -> None:
    bom = {"utf-16-be": codecs.BOM_UTF16_BE, "utf-32-be": codecs.BOM_UTF32_BE}.get(encoding, b"")
    path = tmp_path / "wide.txt"
    path.write_bytes(bom + "Dòng một\n\nĐoạn hai".encode(encoding))
    blocks = TextParser().parse(path).blocks
    assert [b.text for b in blocks] == ["Dòng một", "Đoạn hai"]


def test_csv_parser_reads_a_utf16_file(tmp_path: Path) -> None:
    path = tmp_path / "wide.csv"
    path.write_text("Gói,Giá\nA,10\n", encoding="utf-16")
    (block,) = CsvParser().parse(path).blocks
    assert (
        block.table is not None
        and block.table.header == ("Gói", "Giá")
        and block.table.rows == (("A", "10"),)
    )
