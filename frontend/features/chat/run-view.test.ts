import { describe, expect, it } from "vitest";

import type { ChatMessage } from "./contract";
import { runLines, toolCount, toRunView } from "./run-view";

const locator = { documentId: "d1", kbName: "KB", page: 3, pageCount: 12, blockIds: ["b1"], score: 0.9 };

const message: ChatMessage = {
  id: "a1",
  role: "assistant",
  metadata: { runId: "R-1" },
  parts: [
    {
      type: "reasoning",
      id: "r1",
      text: "Plan",
      state: "done",
      providerMetadata: { thomas: { durationMs: 1200 } },
    },
    {
      type: "dynamic-tool",
      toolName: "kb_search",
      toolCallId: "k1",
      state: "output-available",
      input: { query: "q" },
      output: { chunks: 8, documents: ["a.pdf"] },
      toolMetadata: { line: "kb", source: "Chính sách", durationMs: 600 },
    },
    {
      type: "source-document",
      sourceId: "1",
      mediaType: "application/pdf",
      title: "§2.1",
      filename: "a.pdf",
      providerMetadata: { thomas: locator },
    },
    { type: "text", text: "Answer [1]", state: "done" },
    {
      type: "dynamic-tool",
      toolName: "linear_create_issue",
      toolCallId: "l1",
      state: "approval-requested",
      input: { team: "CS" },
      approval: { id: "ap1" },
      toolMetadata: { line: "mcp", source: "Linear" },
    },
  ],
};

describe("toRunView", () => {
  it("groups stations around the answer text", () => {
    const view = toRunView(message);
    expect(view.segments.map((s) => s.kind)).toEqual(["steps", "text", "steps"]);
    expect(view.runId).toBe("R-1");
  });

  it("carries line, source, timing and approval from the protocol metadata", () => {
    const view = toRunView(message);
    const [first] = view.segments;
    expect(first?.kind === "steps" && first.stations[0]).toMatchObject({
      kind: "reasoning",
      durationMs: 1200,
      live: false,
    });
    expect(view.stations[1]).toMatchObject({
      kind: "tool",
      line: "kb",
      source: "Chính sách",
      state: "done",
      durationMs: 600,
    });
    expect(view.stations[2]).toMatchObject({
      kind: "tool",
      line: "mcp",
      state: "awaiting",
      approvalId: "ap1",
    });
  });

  it("numbers citations in arrival order with their locator", () => {
    const view = toRunView(message);
    expect(view.citations).toEqual([{ n: 1, sourceId: "1", title: "§2.1", fileName: "a.pdf", locator }]);
  });

  it("summarises tool count and the lines touched", () => {
    const view = toRunView(message);
    expect(toolCount(view.stations)).toBe(2);
    expect(runLines(view.stations)).toEqual(["model", "kb", "mcp"]);
  });

  it("never trusts an unknown line from tool metadata", () => {
    const view = toRunView({
      ...message,
      parts: [
        {
          type: "dynamic-tool",
          toolName: "x",
          toolCallId: "x1",
          state: "input-available",
          input: {},
          toolMetadata: { line: "rainbow" },
        },
      ],
    });
    expect(view.stations[0]).toMatchObject({ line: "app", source: "x", state: "running" });
  });
});
