import uuid
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final
from uuid import UUID

from thomas_ragx.parsing.model import Block, BlockType

# Fixed forever: node ids must be equal across processes and re-ingests (never Python hash()).
NODE_NAMESPACE: Final = UUID("6f2b8c1e-3d4a-5b6c-8d7e-9f0a1b2c3d4e")
ROOT_LEVEL: Final = 0
DEFAULT_HEADING_LEVEL: Final = 1


@dataclass(frozen=True)
class OutlineNode:
    node_id: UUID
    parent_id: UUID | None
    position: int
    depth: int
    title: str


@dataclass(frozen=True)
class Outline:
    nodes: tuple[OutlineNode, ...]
    block_nodes: Mapping[str, UUID]
    paths: Mapping[UUID, tuple[str, ...]]


def node_id_for(file_id: UUID, version: int, position: int) -> UUID:
    return uuid.uuid5(NODE_NAMESPACE, f"{file_id}:{version}:{position}")


def build_outline(file_id: UUID, version: int, title: str, blocks: Sequence[Block]) -> Outline:
    """The document tree (doc_nodes): a root per file, one node per heading, nested by heading level."""
    root = OutlineNode(node_id_for(file_id, version, 0), None, 0, 0, title)
    nodes = [root]
    paths: dict[UUID, tuple[str, ...]] = {root.node_id: ()}
    stack: list[tuple[int, OutlineNode]] = [(ROOT_LEVEL, root)]
    block_nodes: dict[str, UUID] = {}
    for block in blocks:
        if block.block_type is BlockType.HEADING:
            level = block.level or DEFAULT_HEADING_LEVEL
            while len(stack) > 1 and stack[-1][0] >= level:
                stack.pop()
            parent = stack[-1][1]
            node = OutlineNode(
                node_id_for(file_id, version, len(nodes)), parent.node_id, len(nodes), len(stack), block.text
            )
            nodes.append(node)
            paths[node.node_id] = (*paths[parent.node_id], block.text)
            stack.append((level, node))
        block_nodes[block.block_id] = stack[-1][1].node_id
    return Outline(tuple(nodes), block_nodes, paths)
