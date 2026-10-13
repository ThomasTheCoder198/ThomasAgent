from pathlib import Path

import pymupdf
import pytest
from docx import Document
from docx.shared import Mm
from pptx import Presentation
from pptx.util import Inches

from thomas_ragx.parsing.page_sizes import read_page_sizes

PIXMAP_WIDTH = 40
PIXMAP_HEIGHT = 20


def test_pdf_sizes_are_per_page_in_points(tmp_path: Path) -> None:
    path = tmp_path / "a.pdf"
    document = pymupdf.open()
    document.new_page(width=595, height=842)
    document.new_page(width=720, height=405)
    document.save(path)
    sizes = read_page_sizes(path, ".pdf")
    assert (sizes.size(1), sizes.size(2)) == ((595.0, 842.0), (720.0, 405.0))
    assert sizes.size(3) is None


def test_pptx_slide_size_is_in_points_for_every_slide(tmp_path: Path) -> None:
    path = tmp_path / "a.pptx"
    deck = Presentation()
    deck.slide_width, deck.slide_height = Inches(10), Inches(7.5)
    deck.save(path)
    sizes = read_page_sizes(path, ".pptx")
    assert sizes.size(1) == sizes.size(5) == pytest.approx((720.0, 540.0))


def test_docx_section_size_is_in_points(tmp_path: Path) -> None:
    path = tmp_path / "a.docx"
    document = Document()
    document.sections[0].page_width, document.sections[0].page_height = Mm(210), Mm(297)
    document.save(path)
    size = read_page_sizes(path, ".docx").size(2)
    assert size == pytest.approx((595.28, 841.89), abs=0.1)


def test_image_size_is_in_pixels(tmp_path: Path) -> None:
    path = tmp_path / "a.png"
    pymupdf.Pixmap(pymupdf.csRGB, pymupdf.IRect(0, 0, PIXMAP_WIDTH, PIXMAP_HEIGHT)).save(path)
    assert read_page_sizes(path, ".png").size(1) == (float(PIXMAP_WIDTH), float(PIXMAP_HEIGHT))


def test_unreadable_source_gives_no_size(tmp_path: Path) -> None:
    path = tmp_path / "broken.pdf"
    path.write_bytes(b"not a pdf")
    assert read_page_sizes(path, ".pdf").size(1) is None
    assert read_page_sizes(path, ".unknown").size(1) is None
