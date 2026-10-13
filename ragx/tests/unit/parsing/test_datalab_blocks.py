from typing import Any

import pytest

from tests.unit.parsing.fixtures import A4_POINTS, datalab_tree, grouped_tree
from thomas_ragx.parsing.datalab_blocks import (
    MAX_TABLE_CELLS,
    MAX_TABLE_COLUMNS,
    list_text_from_html,
    table_from_html,
    text_from_html,
    to_parsed_document,
)
from thomas_ragx.parsing.locators import DATALAB_EXTENSIONS, DATALAB_LOCATOR_KINDS
from thomas_ragx.parsing.model import BlockType, LocatorKind, ParseProvider
from thomas_ragx.parsing.page_sizes import SourcePageSizes

A4 = SourcePageSizes(per_page=(A4_POINTS, A4_POINTS))


def _tree_with(block_type: str, html: str) -> dict[str, Any]:
    leaf = {"block_type": block_type, "html": html, "bbox": [0, 0, 1, 1]}
    return {"children": [{"block_type": "Page", "bbox": [0, 0, 10, 10], "children": [leaf]}]}


def test_datalab_blocks_skip_page_furniture_and_make_a_list_one_block() -> None:
    parsed = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.2, page_sizes=A4)
    assert [b.block_type for b in parsed.blocks] == [
        BlockType.HEADING,
        BlockType.TEXT,
        BlockType.TEXT,
        BlockType.LIST,
        BlockType.TABLE,
    ]
    assert all("Công ty A" not in b.text for b in parsed.blocks)
    assert (parsed.provider, parsed.page_count, parsed.quality) == (ParseProvider.DATALAB, 2, 4.2)
    assert parsed.blocks[3].text == "- Một\n- Hai"  # the local list shape: one item per line


def test_datalab_blocks_read_heading_level_text_and_page_numbers() -> None:
    blocks = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=A4).blocks
    heading, text = blocks[0], blocks[1]
    assert (heading.level, heading.text) == (2, "Điều 1")
    assert text.text == "Hợp đồng có hiệu lực từ ngày ký."
    assert (text.locator.page, blocks[-1].locator.page) == (1, 2)


def test_datalab_pixel_boxes_become_points_per_axis() -> None:
    text = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=A4).blocks[1]
    assert text.locator.kind is LocatorKind.PDF_BBOX
    bbox = text.locator.bbox
    assert bbox is not None
    assert bbox == pytest.approx(
        (193 * 595 / 1596, 242 * 842 / 2240, 1397 * 595 / 1596, 349 * 842 / 2240), abs=0.01
    )
    assert bbox[2] <= A4_POINTS[0] and bbox[3] <= A4_POINTS[1]  # inside the page, so not raw pixels


def test_datalab_boxes_use_the_size_of_the_matching_page() -> None:
    sizes = SourcePageSizes(per_page=(A4_POINTS, (1596.0, 2240.0)))  # page 2 happens to be 1:1
    table = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=sizes).blocks[-1]
    assert table.locator.bbox == pytest.approx((193, 161, 1397, 482), abs=0.01)


def test_datalab_boxes_fall_back_to_the_default_size_for_single_size_formats() -> None:
    sizes = SourcePageSizes(default=A4_POINTS)  # a DOCX has one section size for every page
    blocks = to_parsed_document(datalab_tree(), ".docx", page_count=2, quality=None, page_sizes=sizes).blocks
    assert all(b.locator.bbox is not None for b in blocks)
    assert blocks[0].locator.bbox == pytest.approx(
        (193 * 595 / 1596, 161 * 842 / 2240, 805 * 595 / 1596, 215 * 842 / 2240), abs=0.01
    )


@pytest.mark.parametrize("sizes", [None, SourcePageSizes()])
def test_datalab_without_a_source_size_emits_page_only_locators(sizes: SourcePageSizes | None) -> None:
    blocks = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=sizes).blocks
    assert blocks[1].locator.to_json() == {"kind": "pdf_bbox", "page": 1}
    assert blocks[-1].locator.to_json() == {"kind": "pdf_bbox", "page": 2}


def test_datalab_page_without_a_usable_bbox_gets_page_only_locators() -> None:
    tree = datalab_tree()
    tree["children"][0]["bbox"] = [0, 0, 0, 0]
    blocks = to_parsed_document(tree, ".pdf", page_count=2, quality=4.0, page_sizes=A4).blocks
    assert blocks[1].locator.bbox is None and blocks[-1].locator.bbox is not None


def test_datalab_none_quality_is_kept_as_none() -> None:
    assert to_parsed_document(datalab_tree(), ".docx", page_count=2, quality=None).quality is None


def test_datalab_table_expands_colspan_header_and_rowspan_body_cell() -> None:
    table = (
        to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=A4).blocks[-1].table
    )
    assert table is not None
    assert table.header == ("Gói", "Gói", "Giá")
    assert table.rows == (("A", "x", "10"), ("A", "y", "20"))


def test_table_from_html_gives_every_row_the_header_width() -> None:
    table = table_from_html("<table><tr><th>a</th><th>b</th></tr><tr><td>1</td></tr></table>")
    assert table is not None and table.rows == (("1", ""),)


def test_table_from_html_clips_hostile_spans() -> None:
    table = table_from_html('<table><tr><td rowspan="999999999" colspan="999999999">z</td></tr></table>')
    assert table is not None
    assert len(table.header) <= 1000 and table.rows == ()


def test_table_from_html_without_rows_is_none() -> None:
    assert table_from_html("<table></table>") is None


def test_table_from_html_caps_the_total_width() -> None:
    wide = "".join('<td colspan="1000">c</td>' for _ in range(50))
    table = table_from_html(f"<table><tr>{wide}</tr><tr><td>x</td></tr></table>")
    assert table is not None
    assert len(table.header) <= MAX_TABLE_COLUMNS
    assert {len(row) for row in (table.header, *table.rows)} == {len(table.header)}  # rectangular


def test_table_from_html_bounds_the_total_number_of_cells() -> None:
    row = "<tr>" + '<td colspan="1000" rowspan="1000">c</td>' * 3 + "</tr>"
    table = table_from_html("<table>" + row * 400 + "</table>")
    assert table is not None
    assert (1 + len(table.rows)) * len(table.header) <= MAX_TABLE_CELLS


def test_table_from_html_reads_only_the_outer_table_and_folds_a_nested_one_into_its_cell() -> None:
    inner = "<table><tr><td>x</td><td>y</td></tr></table>"
    outer = f"<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>{inner}</td></tr></table>"
    table = table_from_html(outer)
    assert table is not None
    assert (table.header, table.rows) == (("a", "b"), (("1", "x y"),))


def test_list_html_keeps_one_line_per_item_with_bullets_numbers_and_depth() -> None:
    html = "<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>"
    assert list_text_from_html(html) == "- a\n  - b\n- c"
    assert list_text_from_html("<ol><li>x</li><li>y</li></ol>") == "1. x\n2. y"


def test_list_html_without_a_list_tag_falls_back_to_items_then_text() -> None:
    assert list_text_from_html("<li>solo</li>") == "- solo"
    assert list_text_from_html("<p>plain</p>") == "plain"


def test_code_block_keeps_its_line_breaks_and_indentation() -> None:
    tree = _tree_with("Code", "<pre><code>a = 1\n    b = 2<br>c = 3</code></pre>")
    block = to_parsed_document(tree, ".pdf", page_count=1, quality=None).blocks[0]
    assert (block.block_type, block.text) == (BlockType.CODE, "a = 1\n    b = 2\nc = 3")


def test_text_unwraps_math_and_maps_known_symbols() -> None:
    assert text_from_html("<p>Giá trị <math>x \\leq 10</math>.</p>") == "Giá trị x ≤ 10."


def test_text_keeps_unknown_latex_as_raw_text() -> None:
    assert text_from_html("<p>phím <math>\\mathfrak{K}</math> ok</p>") == "phím \\mathfrak{K} ok"


def test_datalab_blocks_unwrap_math_in_text_blocks() -> None:
    blocks = to_parsed_document(datalab_tree(), ".pdf", page_count=2, quality=4.0, page_sizes=A4).blocks
    assert blocks[2].text == "Giá trị x ≤ 10 và phím \\mathfrak{K}."


def test_datalab_groups_yield_their_children_and_line_children_are_ignored() -> None:
    blocks = to_parsed_document(grouped_tree(), ".pdf", page_count=1, quality=4.0, page_sizes=None).blocks
    assert [b.block_type for b in blocks] == [
        BlockType.TEXT,
        BlockType.TABLE,
        BlockType.CAPTION,
        BlockType.FIGURE,
        BlockType.CAPTION,
    ]
    assert blocks[0].text == "Đoạn một."
    assert [b.text for b in blocks[2:]] == ["Bảng 1. Giá", "Sơ đồ ba bước", "Hình 1. Quy trình"]


def test_datalab_locator_kinds_are_page_boxes_for_every_extension() -> None:
    # S9-1: DOCX and PPTX blocks carry a page and a box too, so there is no docx_paragraph locator here.
    assert (
        set(DATALAB_LOCATOR_KINDS)
        == DATALAB_EXTENSIONS
        == {".pdf", ".png", ".jpg", ".jpeg", ".docx", ".pptx"}
    )
    assert set(DATALAB_LOCATOR_KINDS.values()) == {LocatorKind.PDF_BBOX}
