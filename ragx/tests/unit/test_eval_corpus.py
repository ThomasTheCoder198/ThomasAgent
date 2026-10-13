import json
import unicodedata
from pathlib import Path

EVAL_DIR = Path(__file__).resolve().parents[1] / "eval"
CATEGORIES = {"A1", "A2", "A3", "A4", "A5", "A6"}


def _normalize(text: str) -> str:
    return " ".join(unicodedata.normalize("NFC", text).lower().split())


def test_every_expected_quote_exists_in_its_corpus_file() -> None:
    items = [
        json.loads(line) for line in (EVAL_DIR / "synthetic.jsonl").read_text(encoding="utf-8").splitlines()
    ]
    assert len({item["id"] for item in items}) == len(items)
    for item in items:
        assert item["category"] in CATEGORIES and item["lang"] in {"vi", "en"}
        for expected in item["expected"]:
            source = (EVAL_DIR / "corpus" / expected["file"]).read_text(encoding="utf-8")
            assert _normalize(expected["quote"]) in _normalize(source), item["id"]
    assert sum(1 for i in items if i["lang"] == "vi" and i["expected"]) >= 24
