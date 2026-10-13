from dataclasses import dataclass
from pathlib import Path
from typing import Final

import pymupdf
import structlog
from docx import Document
from pptx import Presentation

from thomas_ragx.parsing.locators import DATALAB_EXTENSIONS
from thomas_ragx.parsing.office import EMU_PER_POINT
from thomas_ragx.parsing.registry import IMAGE_EXTENSIONS

type PageSize = tuple[float, float]

PDF_EXTENSION: Final = ".pdf"
DOCX_EXTENSION: Final = ".docx"
PPTX_EXTENSION: Final = ".pptx"
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class SourcePageSizes:
    """Page sizes of the source file in points (pixels for an image), top-left origin.

    Datalab boxes are in pixels of its own page rendering (ruling S9-3); these sizes turn them into points.
    `per_page` is indexed by 1-based page number; `default` covers formats with one size for every page.
    """

    per_page: tuple[PageSize, ...] = ()
    default: PageSize | None = None

    def size(self, page: int) -> PageSize | None:
        if 1 <= page <= len(self.per_page):
            return self.per_page[page - 1]
        return self.default


def _pdf(path: Path) -> SourcePageSizes:
    with pymupdf.open(path) as document:  # type: ignore[no-untyped-call]  # pymupdf ships partial types
        return SourcePageSizes(
            per_page=tuple((float(page.rect.width), float(page.rect.height)) for page in document)
        )


def _pptx(path: Path) -> SourcePageSizes:
    deck = Presentation(str(path))
    if deck.slide_width is None or deck.slide_height is None:
        return SourcePageSizes()
    return SourcePageSizes(default=(deck.slide_width / EMU_PER_POINT, deck.slide_height / EMU_PER_POINT))


def _docx(path: Path) -> SourcePageSizes:
    # The first section only: a document with differently sized sections gets the first one's scale (S9-3).
    section = Document(str(path)).sections[0]
    if section.page_width is None or section.page_height is None:
        return SourcePageSizes()
    return SourcePageSizes(default=(float(section.page_width.pt), float(section.page_height.pt)))


def _image(path: Path) -> SourcePageSizes:
    pixmap = pymupdf.Pixmap(str(path))  # type: ignore[no-untyped-call]  # pymupdf ships partial types
    return SourcePageSizes(default=(float(pixmap.width), float(pixmap.height)))


def read_page_sizes(path: Path, extension: str) -> SourcePageSizes:
    """Blocking (opens the file): callers run it in a thread. Any failure means 'unknown', never an error."""
    try:
        if extension == PDF_EXTENSION:
            return _pdf(path)
        if extension == PPTX_EXTENSION:
            return _pptx(path)
        if extension == DOCX_EXTENSION:
            return _docx(path)
        if extension in IMAGE_EXTENSIONS and extension in DATALAB_EXTENSIONS:
            return _image(path)
    except Exception as exc:  # the file may be one only Datalab can read; citations then carry the page only
        log.warning("page size unreadable", error_type=type(exc).__name__)
    return SourcePageSizes()
