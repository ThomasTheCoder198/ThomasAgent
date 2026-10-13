from pathlib import Path

from thomas_ragx.parsing.html_files import HtmlParser
from thomas_ragx.parsing.model import BlockType


def test_html_parser_drops_scripts_and_keeps_source_lines(tmp_path: Path) -> None:
    path = tmp_path / "a.html"
    path.write_text(
        "<html><head><script>alert(1)</script></head><body>\n<h1>Giới thiệu</h1>\n"
        "<p>Đoạn <b>một</b>.</p>\n<ul><li>x</li><li>y</li></ul>\n"
        "<table><tr><th>A</th></tr><tr><td>1</td></tr></table>\n</body></html>",
        encoding="utf-8",
    )
    blocks = HtmlParser().parse(path).blocks
    assert [(b.block_type, b.text) for b in blocks][:3] == [
        (BlockType.HEADING, "Giới thiệu"),
        (BlockType.TEXT, "Đoạn một."),
        (BlockType.LIST, "- x\n- y"),
    ]
    assert blocks[0].locator.line_start == 2
    assert all("alert" not in b.text for b in blocks)
    assert blocks[3].table is not None and blocks[3].table.header == ("A",)


def test_html_text_that_sits_only_in_containers_still_becomes_text_blocks(tmp_path: Path) -> None:
    path = tmp_path / "div.html"
    path.write_text(
        "<html><head><title>x</title></head><body>\n<div><div>Chỉ có div <span>và span</span>.</div></div>\n"
        "<section><article>Bài viết</article></section>\n<!-- ghi chú --></body></html>",
        encoding="utf-8",
    )
    blocks = HtmlParser().parse(path).blocks
    assert [(b.block_type, b.text) for b in blocks] == [
        (BlockType.TEXT, "Chỉ có div và span."),
        (BlockType.TEXT, "Bài viết"),
    ]
    assert [b.locator.line_start for b in blocks] == [2, 3]


def test_html_container_text_next_to_block_tags_is_not_duplicated(tmp_path: Path) -> None:
    path = tmp_path / "mixed.html"
    path.write_text(
        "<html><body>\n<div>Mở đầu <b>đậm</b> cuối<h2>Tiêu đề</h2><p>Đoạn</p>đuôi</div>\n"
        "<ul><li>x</li></ul>\n<blockquote><div>trích</div></blockquote></body></html>",
        encoding="utf-8",
    )
    assert [(b.block_type, b.text) for b in HtmlParser().parse(path).blocks] == [
        (BlockType.TEXT, "Mở đầu đậm cuối"),
        (BlockType.HEADING, "Tiêu đề"),
        (BlockType.TEXT, "Đoạn"),
        (BlockType.TEXT, "đuôi"),
        (BlockType.LIST, "- x"),
        (BlockType.TEXT, "trích"),
    ]


DEEP = 2000  # html.parser nests unclosed tags, so a malformed legacy page can be thousands deep


def test_html_nested_two_thousand_levels_deep_does_not_overflow_the_stack(tmp_path: Path) -> None:
    path = tmp_path / "deep.html"
    path.write_text("<div>" * DEEP + "<p>deep text</p>" + "</div>" * DEEP, encoding="utf-8")
    assert [(b.block_type, b.text) for b in HtmlParser().parse(path).blocks] == [
        (BlockType.TEXT, "deep text")
    ]


def test_html_two_thousand_unclosed_inline_tags_do_not_overflow_the_stack(tmp_path: Path) -> None:
    path = tmp_path / "unclosed.html"
    path.write_text("<font>" * DEEP + "head <p>t</p>tail", encoding="utf-8")
    assert [b.text for b in HtmlParser().parse(path).blocks] == ["head", "t", "tail"]


def test_html_line_breaks_and_form_controls_separate_words(tmp_path: Path) -> None:
    path = tmp_path / "br.html"
    path.write_text(
        "<div>dòng một<br>dòng hai</div>\n<p>a<br/>b</p>\n"
        "<div><label>Tên</label><select><option>Một</option><option>Hai</option></select>"
        "<button>Gửi</button></div>\n<pre>x<br>y</pre>",
        encoding="utf-8",
    )
    assert [(b.block_type, b.text) for b in HtmlParser().parse(path).blocks] == [
        (BlockType.TEXT, "dòng một dòng hai"),
        (BlockType.TEXT, "a b"),
        (BlockType.TEXT, "Tên Một Hai Gửi"),
        (BlockType.CODE, "x\ny"),
    ]
