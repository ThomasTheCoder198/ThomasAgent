import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
FRONTEND_CSS = ROOT / "frontend" / "app" / "globals.css"
RAGX_CSS = ROOT / "ragx" / "src" / "thomas_ragx" / "ui" / "static" / "ragx.css"
BLOCK = re.compile(r'^(:root|\[data-theme="dark"\]) \{\n(.*?)\n\}', re.MULTILINE | re.DOTALL)


def _token_blocks(path: Path) -> dict[str, str]:
    return {selector: body for selector, body in BLOCK.findall(path.read_text(encoding="utf-8"))}


def test_ragx_ui_uses_the_frontend_tokens_verbatim() -> None:
    frontend, ragx = _token_blocks(FRONTEND_CSS), _token_blocks(RAGX_CSS)
    assert set(frontend) == {":root", '[data-theme="dark"]'}
    assert ragx == frontend, "copy the :root and dark token blocks from frontend/app/globals.css unchanged"
