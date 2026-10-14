from dataclasses import dataclass
from typing import Any, Final

from pydantic import BaseModel

from thomas_ragx.auth.caller import (
    TOOL_KB_LIST_DOCUMENTS,
    TOOL_KB_OUTLINE,
    TOOL_KB_READ_SECTION,
    TOOL_KB_SEARCH,
)
from thomas_ragx.retrieval.tool_dto import (
    KbListDocumentsInput,
    KbOutlineInput,
    KbReadSectionInput,
    KbSearchInput,
)

FAILURE_MODE_SKIP: Final = "skip"  # the agent answers without KB evidence rather than aborting (spec §13)


@dataclass(frozen=True)
class ToolDefinition:
    name: str
    description: str
    input_model: type[BaseModel]


TOOLS: Final = (
    ToolDefinition(
        TOOL_KB_SEARCH,
        "Search the Agent's Knowledge Base. Returns passages split into labelled blocks "
        "([n.k]) with locators; cite the labels you use.",
        KbSearchInput,
    ),
    ToolDefinition(
        TOOL_KB_OUTLINE, "List the section tree of one document (titles and short summaries).", KbOutlineInput
    ),
    ToolDefinition(
        TOOL_KB_READ_SECTION,
        "Read one section (and its subsections) of a document in full.",
        KbReadSectionInput,
    ),
    ToolDefinition(
        TOOL_KB_LIST_DOCUMENTS,
        "List the documents in the Knowledge Base, optionally filtered by name or type.",
        KbListDocumentsInput,
    ),
)
TOOL_NAMES: Final = tuple(tool.name for tool in TOOLS)


def manifest_tools(timeout_ms: int) -> list[dict[str, Any]]:
    """App-contract tool entries (ruling C8); the widget flag `public` stays false until M7."""
    return [
        {
            "name": tool.name,
            "description": tool.description,
            "inputSchema": tool.input_model.model_json_schema(by_alias=True),
            "timeoutMs": timeout_ms,
            "failureMode": FAILURE_MODE_SKIP,
            "public": False,
        }
        for tool in TOOLS
    ]
