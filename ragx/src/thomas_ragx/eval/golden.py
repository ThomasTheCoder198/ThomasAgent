import json
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Final

CATEGORIES: Final = frozenset({"A1", "A2", "A3", "A4", "A5", "A6"})
LANGS: Final = frozenset({"vi", "en"})
NORMAL_FORM: Final = "NFC"


def normalize(text: str) -> str:
    return " ".join(unicodedata.normalize(NORMAL_FORM, text).lower().split())


@dataclass(frozen=True)
class Expected:
    file: str
    quote: str


@dataclass(frozen=True)
class GoldenItem:
    id: str
    lang: str
    category: str
    question: str
    expected: tuple[Expected, ...]


def load_golden(path: Path) -> list[GoldenItem]:
    """JSONL, one item per line (format in tests/eval/README.md); the owner's real set shares it."""
    items = []
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        raw = json.loads(line)
        if raw["category"] not in CATEGORIES or raw["lang"] not in LANGS:
            raise ValueError(f"line {number}: unknown category or lang")
        expected = tuple(Expected(e["file"], e["quote"]) for e in raw["expected"])
        items.append(GoldenItem(raw["id"], raw["lang"], raw["category"], raw["question"], expected))
    return items
