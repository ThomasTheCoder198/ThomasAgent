import { describe, expect, it } from "vitest";

import type { AgentDetail } from "./contract";
import { canSave, diffDraft, draftErrors, fromDetail, isDirty } from "./draft";

const agent: AgentDetail = {
  id: "a1",
  name: "Pháp chế",
  description: "Mô tả",
  instructions: "Trả lời có nguồn.",
  kbId: "kb-1",
  toolSources: [
    {
      id: "s1",
      line: "mcp",
      kind: "mcp",
      name: "Linear",
      tools: [
        { id: "t1", name: "create_issue", title: "Tạo issue", description: "", risky: true, policy: "ask" },
        { id: "t2", name: "search", title: "Tìm", description: "", risky: false, policy: "auto" },
      ],
    },
  ],
  modelOverrides: { "chat.fast": "m-fast" },
  contextFiles: [],
  updatedAt: "",
};

describe("agent draft", () => {
  it("is clean right after loading", () => {
    const draft = fromDetail(agent);
    expect(diffDraft(agent, draft)).toEqual({});
    expect(isDirty(agent, draft)).toBe(false);
  });

  it("sends only the fields that changed", () => {
    const draft = { ...fromDetail(agent), instructions: "Mới", kbId: "kb-2" };
    expect(diffDraft(agent, draft)).toEqual({ instructions: "Mới", kbId: "kb-2" });
  });

  it("sends only the tool policies that changed", () => {
    const base = fromDetail(agent);
    const draft = { ...base, policies: { ...base.policies, t1: "auto" as const } };
    expect(diffDraft(agent, draft)).toEqual({ toolPolicies: { t1: "auto" } });
  });

  it("clears a model override with null and sets a new one", () => {
    const draft = { ...fromDetail(agent), overrides: { "chat.default": "m-big" } };
    expect(diffDraft(agent, draft)).toEqual({
      modelOverrides: { "chat.default": "m-big", "chat.fast": null },
    });
  });

  it("treats a trimmed-equal name as unchanged", () => {
    const draft = { ...fromDetail(agent), name: "  Pháp chế " };
    expect(isDirty(agent, draft)).toBe(false);
  });

  it("flags an empty or whitespace-only name and never sends it", () => {
    for (const name of ["", "   "]) {
      const draft = { ...fromDetail(agent), name };
      expect(draftErrors(draft)).toEqual({ name: "required" });
      expect(canSave(agent, draft)).toBe(false);
    }
  });

  it("normalises a whitespace-only description to empty", () => {
    const draft = { ...fromDetail(agent), description: "   " };
    expect(diffDraft(agent, draft)).toEqual({ description: "" });
  });

  it("has an empty diff once every edit is reverted", () => {
    const base = fromDetail(agent);
    const edited = {
      ...base,
      instructions: "Khác",
      kbId: "kb-2",
      policies: { ...base.policies, t2: "off" as const },
    };
    expect(isDirty(agent, edited)).toBe(true);
    const reverted = { ...edited, instructions: base.instructions, kbId: base.kbId, policies: base.policies };
    expect(diffDraft(agent, reverted)).toEqual({});
    expect(canSave(agent, reverted)).toBe(false);
  });

  it("can save a valid change", () => {
    expect(canSave(agent, { ...fromDetail(agent), name: "Pháp chế mới" })).toBe(true);
  });
});
