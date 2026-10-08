import json
import re
from importlib.resources import files

TEMPLATE_KEY = re.compile(r"""t\(['"]([a-z_.]+)['"]\)""")


def _messages(lang: str) -> dict[str, str]:
    raw = files("thomas_ragx.ui").joinpath("messages", f"{lang}.json").read_text(encoding="utf-8")
    return dict(json.loads(raw))


def test_vietnamese_and_english_have_the_same_keys() -> None:
    assert _messages("vi").keys() == _messages("en").keys()


def test_every_key_used_by_a_template_exists() -> None:
    used: set[str] = set()
    for template in files("thomas_ragx.ui").joinpath("templates").iterdir():
        used |= set(TEMPLATE_KEY.findall(template.read_text(encoding="utf-8")))
    assert used <= _messages("vi").keys()
