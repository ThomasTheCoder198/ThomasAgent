from thomas_ragx.retrieval.tool_catalog import TOOL_NAMES, manifest_tools


def test_manifest_lists_the_four_tools_with_closed_schemas() -> None:
    tools = manifest_tools(20000)
    assert (
        [t["name"] for t in tools]
        == list(TOOL_NAMES)
        == [
            "kb_search",
            "kb_outline",
            "kb_read_section",
            "kb_list_documents",
        ]
    )
    for tool in tools:
        assert tool["inputSchema"]["additionalProperties"] is False
        assert (tool["timeoutMs"], tool["failureMode"], tool["public"]) == (20000, "skip", False)
        assert "kbId" not in tool["inputSchema"]["properties"]
    assert tools[0]["inputSchema"]["required"] == ["query"]
