import unicodedata

import pytest

from thomas_ragx.kb.upload import check_declared_length, content_type_for, normalize_file_name
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        (unicodedata.normalize("NFD", "Tài liệu.pdf"), "Tài liệu.pdf"),
        ("../../etc/passwd.txt", "passwd.txt"),
        ("C:\\Users\\me\\report.docx", "report.docx"),
        ("  spaced.md  ", "spaced.md"),
    ],
)
def test_normalize_file_name(raw: str, expected: str) -> None:
    assert normalize_file_name(raw) == expected
    assert unicodedata.is_normalized("NFC", normalize_file_name(raw))


@pytest.mark.parametrize(
    "raw", ["", "   ", "..", "folder/", "a" * 252 + ".pdf", "nul\x00.txt", "line\n.txt", "tab\t.txt"]
)
def test_normalize_file_name_rejects_empty_or_too_long(raw: str) -> None:
    with pytest.raises(AppError) as failure:
        normalize_file_name(raw)
    assert failure.value.code is ErrorCode.VALIDATION_FAILED


def test_content_type_comes_from_the_extension_not_the_client() -> None:
    assert content_type_for("Báo cáo.PDF") == "application/pdf"
    assert (
        content_type_for("sheet.xlsx") == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    )
    with pytest.raises(AppError) as failure:
        content_type_for("setup.exe")
    assert failure.value.code is ErrorCode.UNSUPPORTED_MEDIA_TYPE


@pytest.mark.parametrize(
    ("value", "code"),
    [
        (None, "VALIDATION_FAILED"),
        ("abc", "VALIDATION_FAILED"),
        ("-1", "VALIDATION_FAILED"),
        ("2001", "PAYLOAD_TOO_LARGE"),
    ],
)
def test_declared_length_is_required_and_bounded(value: str | None, code: str) -> None:
    with pytest.raises(AppError) as failure:
        check_declared_length(value, max_bytes=1000, overhead_bytes=1000)
    assert failure.value.code.value == code


def test_declared_length_within_bounds_passes() -> None:
    check_declared_length("2000", max_bytes=1000, overhead_bytes=1000)
