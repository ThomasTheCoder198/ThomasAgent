from collections.abc import Mapping
from typing import Final

from thomas_ragx.parsing.model import LocatorKind

# Spike S9 (ruling S9-1): DOCX and PPTX blocks from Datalab carry a page and a box like a PDF's, and the
# JSON has no line level, so every Datalab extension cites a page box.
DATALAB_LOCATOR_KINDS: Final[Mapping[str, LocatorKind]] = {
    ".pdf": LocatorKind.PDF_BBOX,
    ".png": LocatorKind.PDF_BBOX,
    ".jpg": LocatorKind.PDF_BBOX,
    ".jpeg": LocatorKind.PDF_BBOX,
    ".docx": LocatorKind.PDF_BBOX,
    ".pptx": LocatorKind.PDF_BBOX,
}
DATALAB_EXTENSIONS: Final = frozenset(DATALAB_LOCATOR_KINDS)
