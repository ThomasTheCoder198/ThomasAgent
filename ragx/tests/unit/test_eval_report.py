import json
from pathlib import Path

import pytest

from thomas_ragx.eval.golden import load_golden, normalize
from thomas_ragx.eval.report import percentile


def test_percentile_is_nearest_rank() -> None:
    values = [10.0, 20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0]
    assert (percentile(values, 0.5), percentile(values, 0.95)) == (50.0, 100.0)
    assert percentile([7.0], 0.95) == 7.0


def test_normalize_is_nfc_lowercase_and_collapses_spaces() -> None:
    # "Hợp đồng" typed with combining marks; ơ needs the horn (U+031B) as well as the dot below.
    decomposed = "Hợp  đồng"
    assert normalize(decomposed) == "hợp đồng"


def test_load_golden_reads_jsonl_and_skips_blank_lines(tmp_path: Path) -> None:
    item = {
        "id": "x-1",
        "lang": "en",
        "category": "A2",
        "question": "q?",
        "expected": [{"file": "a.md", "quote": "text"}],
    }
    path = tmp_path / "set.jsonl"
    path.write_text(json.dumps(item) + "\n\n", encoding="utf-8")
    (loaded,) = load_golden(path)
    assert (loaded.id, loaded.category, loaded.expected[0].file) == ("x-1", "A2", "a.md")


def test_load_golden_rejects_an_unknown_category(tmp_path: Path) -> None:
    path = tmp_path / "set.jsonl"
    path.write_text(
        json.dumps({"id": "x", "lang": "en", "category": "Z9", "question": "q", "expected": []}),
        encoding="utf-8",
    )
    with pytest.raises(ValueError, match="line 1"):
        load_golden(path)
