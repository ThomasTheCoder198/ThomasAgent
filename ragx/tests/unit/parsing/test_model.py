from thomas_ragx.parsing.model import (
    BlockCollector,
    BlockType,
    Locator,
    LocatorKind,
    TableData,
    docx_paragraph,
    pdf_bbox,
    sheet_range,
    table_text,
    text_lines,
)


def test_locator_json_omits_empty_fields_and_uses_contract_keys() -> None:
    assert pdf_bbox(3, (1.0, 2.0, 3.0, 4.0)).to_json() == {
        "kind": "pdf_bbox",
        "page": 3,
        "bbox": [1.0, 2.0, 3.0, 4.0],
    }
    assert sheet_range("Q1", "A1:C9").to_json() == {"kind": "sheet_range", "sheet": "Q1", "range": "A1:C9"}
    assert text_lines(4, 7).to_json() == {"kind": "text_lines", "lineStart": 4, "lineEnd": 7}
    assert docx_paragraph(12).to_json() == {"kind": "docx_paragraph", "paragraph": 12}
    assert Locator(LocatorKind.PDF_BBOX, page=2).to_json() == {"kind": "pdf_bbox", "page": 2}


def test_collector_numbers_blocks_in_order_and_skips_blank_text() -> None:
    collector = BlockCollector()
    collector.add(BlockType.HEADING, " Tiêu đề ", text_lines(1, 1), level=1)
    collector.add(BlockType.TEXT, "   ", text_lines(2, 2))
    collector.add(BlockType.TEXT, "Nội dung", text_lines(3, 3))
    assert [(b.block_id, b.text) for b in collector.blocks()] == [
        ("b-0001", "Tiêu đề"),
        ("b-0002", "Nội dung"),
    ]


def test_table_text_is_a_markdown_table() -> None:
    table = TableData(header=("Tên", "Giá"), rows=(("A", "1"), ("B|C", "2")))
    assert table_text(table) == "| Tên | Giá |\n| --- | --- |\n| A | 1 |\n| B\\|C | 2 |"


def test_collector_strips_nul_characters_that_postgres_text_and_jsonb_reject() -> None:
    collector = BlockCollector()
    collector.add(BlockType.TEXT, "ab\x00cd", text_lines(1, 1))
    collector.add(
        BlockType.TABLE, "", text_lines(2, 3), table=TableData(header=("A\x00",), rows=(("1\x002",),))
    )
    text_block, table_block = collector.blocks()
    assert text_block.text == "abcd"
    assert table_block.table is not None
    assert (table_block.table.header, table_block.table.rows) == (("A",), (("12",),))
    assert "\x00" not in table_block.text
