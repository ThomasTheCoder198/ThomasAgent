import uuid

from thomas_ragx.ingest.outline import build_outline, node_id_for
from thomas_ragx.parsing.model import BlockCollector, BlockType, text_lines

FILE_ID = uuid.UUID("00000000-0000-0000-0000-0000000000aa")


def _blocks() -> tuple:
    collector = BlockCollector()
    collector.add(BlockType.TEXT, "Lời nói đầu", text_lines(1, 1))
    collector.add(BlockType.HEADING, "Chương 1", text_lines(2, 2), level=1)
    collector.add(BlockType.TEXT, "A", text_lines(3, 3))
    collector.add(BlockType.HEADING, "Mục 1.1", text_lines(4, 4), level=2)
    collector.add(BlockType.TEXT, "B", text_lines(5, 5))
    collector.add(BlockType.HEADING, "Chương 2", text_lines(6, 6), level=1)
    return collector.blocks()


def test_outline_nests_headings_and_assigns_every_block_a_node() -> None:
    outline = build_outline(FILE_ID, 1, "hop-dong.md", _blocks())
    titles = [(n.title, n.depth) for n in outline.nodes]
    assert titles == [("hop-dong.md", 0), ("Chương 1", 1), ("Mục 1.1", 2), ("Chương 2", 1)]
    root, chapter, section, second = outline.nodes
    assert (chapter.parent_id, section.parent_id, second.parent_id) == (
        root.node_id,
        chapter.node_id,
        root.node_id,
    )
    assert outline.block_nodes["b-0001"] == root.node_id
    assert outline.block_nodes["b-0005"] == section.node_id
    assert outline.paths[section.node_id] == ("Chương 1", "Mục 1.1")


def test_node_ids_are_deterministic_across_runs() -> None:
    first = build_outline(FILE_ID, 1, "a", _blocks())
    second = build_outline(FILE_ID, 1, "a", _blocks())
    assert [n.node_id for n in first.nodes] == [n.node_id for n in second.nodes]
    assert node_id_for(FILE_ID, 2, 0) != node_id_for(FILE_ID, 1, 0)
