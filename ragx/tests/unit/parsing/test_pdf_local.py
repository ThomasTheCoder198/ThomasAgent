from pathlib import Path

import pymupdf

from thomas_ragx.parsing.model import BlockType, LocatorKind
from thomas_ragx.parsing.pdf_local import PdfParser


def _pdf(path: Path) -> None:
    document = pymupdf.open()
    page = document.new_page()
    page.insert_text((72, 80), "Chapter One", fontsize=24)
    page.insert_text((72, 140), "Body text of the first chapter.", fontsize=11)
    page.insert_text((72, 160), "More body text here.", fontsize=11)
    document.new_page().insert_text((72, 80), "Second page body.", fontsize=11)
    document.save(path)


def test_pdf_parser_gives_page_boxes_and_detects_headings_by_font_size(tmp_path: Path) -> None:
    path = tmp_path / "a.pdf"
    _pdf(path)
    parsed = PdfParser().parse(path)
    assert parsed.page_count == 2
    assert parsed.blocks[0].block_type is BlockType.HEADING and parsed.blocks[0].text == "Chapter One"
    assert {b.locator.kind for b in parsed.blocks} == {LocatorKind.PDF_BBOX}
    assert parsed.blocks[-1].locator.page == 2
    x0, y0, x1, y1 = parsed.blocks[0].locator.bbox or (0, 0, 0, 0)
    assert x0 < x1 and y0 < y1


def test_pdf_spans_of_one_line_are_joined_without_inventing_spaces(tmp_path: Path) -> None:
    path = tmp_path / "spans.pdf"
    document = pymupdf.open()
    page = document.new_page()
    width = pymupdf.get_text_length("Hel", fontsize=11)
    page.insert_text((72, 80), "Hel", fontsize=11)
    page.insert_text((72 + width, 80), "lo world", fontsize=11, fontname="hebo")  # a bold span
    document.save(path)
    assert PdfParser().parse(path).blocks[0].text == "Hello world"


def test_pdf_parser_without_text_yields_no_blocks(tmp_path: Path) -> None:
    path = tmp_path / "blank.pdf"
    document = pymupdf.open()
    document.new_page()
    document.save(path)
    assert PdfParser().parse(path).blocks == ()
