import { describe, expect, it } from "vitest";

import type { Agent } from "@/lib/core/types";

import { canSave, diffDraft, draftErrors, fromAgent, isDirty } from "./draft";

const agent: Agent = {
  id: "a1",
  name: "Pháp chế",
  description: "Mô tả",
  instructions: "Trả lời có nguồn.",
  kbId: "kb-1",
  kbName: "KB",
  kbMissing: false,
  modelOverrides: { "chat.fast": "m-fast" },
  createdAt: "x",
  updatedAt: "x",
};

describe("agent draft", () => {
  it("is clean right after loading", () => {
    const draft = fromAgent(agent);
    expect(diffDraft(agent, draft)).toEqual({});
    expect(isDirty(agent, draft)).toBe(false);
  });

  it("sends only the fields that changed", () => {
    const draft = { ...fromAgent(agent), instructions: "Mới", kbId: "kb-2" };
    expect(diffDraft(agent, draft)).toEqual({ instructions: "Mới", kbId: "kb-2" });
  });

  it("clears a model override with null and sets a new one", () => {
    const draft = { ...fromAgent(agent), overrides: { "chat.default": "m-big" } };
    expect(diffDraft(agent, draft)).toEqual({
      modelOverrides: { "chat.default": "m-big", "chat.fast": null },
    });
  });

  it("treats a trimmed-equal name as unchanged", () => {
    const draft = { ...fromAgent(agent), name: "  Pháp chế " };
    expect(isDirty(agent, draft)).toBe(false);
  });

  it("flags an empty or whitespace-only name and never sends it", () => {
    for (const name of ["", "   "]) {
      const draft = { ...fromAgent(agent), name };
      expect(draftErrors(draft)).toEqual({ name: "required" });
      expect(canSave(agent, draft)).toBe(false);
    }
  });

  it("normalises a whitespace-only description to empty", () => {
    const draft = { ...fromAgent(agent), description: "   " };
    expect(diffDraft(agent, draft)).toEqual({ description: "" });
  });

  it("has an empty diff once every edit is reverted", () => {
    const base = fromAgent(agent);
    const edited = { ...base, instructions: "Khác", kbId: "kb-2" };
    expect(isDirty(agent, edited)).toBe(true);
    const reverted = { ...edited, instructions: base.instructions, kbId: base.kbId };
    expect(diffDraft(agent, reverted)).toEqual({});
    expect(canSave(agent, reverted)).toBe(false);
  });

  it("can save a valid change", () => {
    expect(canSave(agent, { ...fromAgent(agent), name: "Pháp chế mới" })).toBe(true);
  });

  it("refuses names and instructions longer than core accepts", () => {
    const draft = { ...fromAgent(agent), name: "x".repeat(81), instructions: "y".repeat(8001) };
    expect(draftErrors(draft)).toEqual({ name: "tooLong", instructions: "tooLong" });
  });

  it("clears an override with null", () => {
    const withOverride = { ...agent, modelOverrides: { "chat.fast": "m2" } };
    const draft = { ...fromAgent(withOverride), overrides: {} };
    expect(diffDraft(withOverride, draft)).toEqual({ modelOverrides: { "chat.fast": null } });
  });
});
