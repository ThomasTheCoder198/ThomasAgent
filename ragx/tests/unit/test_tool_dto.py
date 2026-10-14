from datetime import UTC, datetime
from uuid import uuid4

import pytest
from pydantic import ValidationError

from thomas_ragx.retrieval.tool_dto import KbSearchFilters, document_response


def test_doc_type_filters_are_stored_the_way_ingest_writes_them() -> None:
    filters = KbSearchFilters.model_validate({"docTypes": ["PDF", ".pdf", " .Md "]})
    assert filters.doc_types == ["pdf", "pdf", "md"]


@pytest.mark.parametrize("bad", [".", "", "p.df", "a b", "p%f"])
def test_doc_type_filters_reject_non_alphanumeric_entries(bad: str) -> None:
    with pytest.raises(ValidationError):
        KbSearchFilters.model_validate({"docTypes": [bad]})


def test_document_response_reports_a_lowercase_doc_type() -> None:
    row = {
        "id": uuid4(),
        "name": "Bao-Cao.PDF",
        "version": 1,
        "status": "ready",
        "page_count": 2,
        "created_at": datetime.now(UTC),
    }
    assert document_response(row).doc_type == "pdf"  # type: ignore[arg-type]
