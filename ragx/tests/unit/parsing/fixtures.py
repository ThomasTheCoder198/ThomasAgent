from typing import Any

# An A4 page (595 x 842 pt) rendered by Datalab: page-image pixels, about 2.68x the point size (spike S9).
PAGE_PX_WIDTH = 1596
PAGE_PX_HEIGHT = 2240
A4_POINTS = (595.0, 842.0)


def _leaf(block_type: str, block_id: str, html: str, page: int, bbox: list[int]) -> dict[str, Any]:
    """A leaf as S9 measured it: page, bbox, polygon, markdown null and no `children` key."""
    x0, y0, x1, y1 = bbox
    return {
        "id": block_id,
        "block_type": block_type,
        "html": html,
        "page": page,
        "bbox": bbox,
        "polygon": [[x0, y0], [x1, y0], [x1, y1], [x0, y1]],
        "section_hierarchy": {},
        "images": {},
        "markdown": None,
        "inference_failed": False,
        "metadata": None,
    }


def _page(
    number: int, children: list[dict[str, Any]], hierarchy: dict[str, str] | None = None
) -> dict[str, Any]:
    return {
        "id": f"/page/{number}/Page/{number}",
        "block_type": "Page",
        "html": "",
        "bbox": [0, 0, PAGE_PX_WIDTH, PAGE_PX_HEIGHT],
        "polygon": [[0, 0], [PAGE_PX_WIDTH, 0], [PAGE_PX_WIDTH, PAGE_PX_HEIGHT], [0, PAGE_PX_HEIGHT]],
        "section_hierarchy": hierarchy or {},
        "children": children,
    }


def _root(pages: list[dict[str, Any]]) -> dict[str, Any]:
    """The S9 root has no block_type and no id."""
    return {"children": pages, "metadata": {"page_stats": [], "failed_pages": []}}


TABLE_HTML = (
    "<table>"
    '<tr><th colspan="2">Gói</th><th>Giá</th></tr>'
    '<tr><td rowspan="2">A</td><td>x</td><td>10</td></tr>'
    "<tr><td>y</td><td>20</td></tr>"
    "</table>"
)


def datalab_tree() -> dict[str, Any]:
    """Synthetic S9-shaped tree: furniture, a heading, text with a tag and math, a list, a table."""
    page0 = _page(
        0,
        [
            _leaf("PageHeader", "/page/0/PageHeader/0", "<p>Công ty A</p>", 0, [0, 0, 1596, 80]),
            _leaf("SectionHeader", "/page/0/SectionHeader/1", "<h2>Điều 1</h2>", 0, [193, 161, 805, 215]),
            _leaf(
                "Text",
                "/page/0/Text/2",
                "<p>Hợp đồng có hiệu lực <b>từ ngày ký</b>.</p>",
                0,
                [193, 242, 1397, 349],
            ),
            _leaf(
                "Text",
                "/page/0/Text/3",
                "<p>Giá trị <math>x \\leq 10</math> và phím <math>\\mathfrak{K}</math>.</p>",
                0,
                [193, 400, 1397, 450],
            ),
            _leaf(
                "ListGroup",
                "/page/0/ListGroup/4",
                '<ul style="list-style-type: none"><li>Một</li><li>Hai</li></ul>',
                0,
                [193, 480, 1397, 600],
            ),
        ],
    )
    page1 = _page(
        1,
        [
            _leaf("Table", "/page/1/Table/0", TABLE_HTML, 1, [193, 161, 1397, 482]),
            _leaf("PageFooter", "/page/1/PageFooter/1", "<p>2</p>", 1, [0, 2160, 1596, 2240]),
        ],
        hierarchy={"2": "/page/0/SectionHeader/1"},
    )
    return _root([page0, page1])


def grouped_tree() -> dict[str, Any]:
    """Robustness cases S9 did not exercise: groups with captions, figures and Line children."""
    text = _leaf("Text", "/page/0/Text/2", "<p>Đoạn một.</p>", 0, [193, 90, 1397, 130])
    text["children"] = [_leaf("Line", "/page/0/Line/9", "<p>Đoạn một.</p>", 0, [193, 90, 1397, 100])]
    table_group = _leaf("TableGroup", "/page/0/TableGroup/3", "", 0, [193, 160, 1397, 500])
    table_group["children"] = [
        _leaf(
            "Table",
            "/page/0/Table/4",
            "<table><tr><th>Gói</th><th>Giá</th></tr><tr><td>A</td><td>10</td></tr></table>",
            0,
            [193, 160, 1397, 440],
        ),
        _leaf("Caption", "/page/0/Caption/5", "<p>Bảng 1. Giá</p>", 0, [193, 450, 800, 490]),
    ]
    figure_group = _leaf("FigureGroup", "/page/0/FigureGroup/6", "", 0, [193, 520, 1397, 900])
    figure_group["children"] = [
        _leaf("Figure", "/page/0/Figure/7", "<p>Sơ đồ ba bước</p>", 0, [193, 520, 1397, 860]),
        _leaf("Caption", "/page/0/Caption/8", "<p>Hình 1. Quy trình</p>", 0, [193, 870, 800, 900]),
    ]
    return _root([_page(0, [text, table_group, figure_group])])
