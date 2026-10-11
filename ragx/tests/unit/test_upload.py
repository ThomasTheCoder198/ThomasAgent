import unicodedata

import pytest

from thomas_ragx.kb.upload import content_type_for, normalize_file_name
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
    assert normalize_file_name(raw, "name") == expected
    assert unicodedata.is_normalized("NFC", normalize_file_name(raw, "name"))


@pytest.mark.parametrize(
    "raw", ["", "   ", "..", "folder/", "a" * 252 + ".pdf", "nul\x00.txt", "line\n.txt", "tab\t.txt"]
)
def test_normalize_file_name_rejects_empty_or_too_long(raw: str) -> None:
    with pytest.raises(AppError) as failure:
        normalize_file_name(raw, "name")
    assert failure.value.code is ErrorCode.VALIDATION_FAILED
    assert failure.value.details == {"fields": [{"field": "name", "code": "INVALID_NAME"}]}


def test_content_type_comes_from_the_extension_not_the_client() -> None:
    assert content_type_for("Báo cáo.PDF") == "application/pdf"
    assert (
        content_type_for("sheet.xlsx") == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    )
    with pytest.raises(AppError) as failure:
        content_type_for("setup.exe")
    assert failure.value.code is ErrorCode.UNSUPPORTED_MEDIA_TYPE
