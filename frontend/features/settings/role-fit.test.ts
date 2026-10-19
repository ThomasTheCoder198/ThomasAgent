import { describe, expect, it } from "vitest";

import type { Model } from "@/features/registry/types";

import { fitsRole, SETTINGS_ROLES } from "./role-fit";

const model = (capabilities: Model["capabilities"]): Model => ({
  id: "m",
  providerId: "p",
  modelRef: "x/y",
  displayName: "Y",
  capabilities,
  contextWindow: null,
  source: "sync",
});

describe("fitsRole (mirrors core RequirementsForRole)", () => {
  it("needs chat and tools for the main chat role", () => {
    expect(fitsRole(model(["chat", "tools"]), "chat.default")).toBe(true);
    expect(fitsRole(model(["chat"]), "chat.default")).toBe(false);
  });

  it("takes any chat model for the fast role, vision for vision, decision for decision", () => {
    expect(fitsRole(model(["chat"]), "chat.fast")).toBe(true);
    expect(fitsRole(model(["chat", "vision"]), "vision")).toBe(true);
    expect(fitsRole(model(["decision"]), "decision")).toBe(true);
    expect(fitsRole(model(["chat"]), "decision")).toBe(false);
  });

  it("treats a model with unknown capabilities as not fitting", () => {
    expect(fitsRole(model([]), "chat.fast")).toBe(false);
  });

  it("lists the core roles this page sets, in display order", () => {
    expect(SETTINGS_ROLES).toEqual(["chat.default", "chat.fast", "decision", "vision"]);
  });
});
