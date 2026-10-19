import { describe, expect, it } from "vitest";

import { chosenModelId, dropChange, initialProviderId, inheritedModelId, roleView } from "./role-view";

const platform = { scope: "platform" } as const;
const org = { scope: "org", orgId: "o1" } as const;
const base = { role: "chat.fast", chosenId: null, modelExists: false, overriding: false, ownMode: false };

describe("role view", () => {
  it("an org takes only its own source as its model; the rest is inherited", () => {
    expect(chosenModelId(org, { modelId: "m1", source: "platform" }, undefined)).toBeNull();
    expect(chosenModelId(org, { modelId: "m2", source: "org" }, undefined)).toBe("m2");
    expect(chosenModelId(org, { modelId: "m2", source: "org" }, { kind: "clear" })).toBeNull();
    expect(
      chosenModelId(platform, { modelId: "m1", source: "platform" }, { kind: "set", modelId: "m3" }),
    ).toBe("m3");
  });

  it("names what an org inherits, even when it is clearing its own override", () => {
    expect(inheritedModelId({ modelId: "m1", source: "platform" }, "m1")).toBe("m1");
    expect(inheritedModelId({ modelId: "m2", source: "org" }, "m1")).toBe("m1");
    expect(inheritedModelId({ modelId: null, source: "none" }, undefined)).toBeNull();
  });

  it("only sends a clear when there is a stored model to clear", () => {
    expect(dropChange(platform, { modelId: "m1", source: "platform" })).toEqual({ kind: "clear" });
    expect(dropChange(platform, { modelId: null, source: "none" })).toBeUndefined();
    expect(dropChange(org, { modelId: "m1", source: "platform" })).toBeUndefined();
    expect(dropChange(org, { modelId: "m2", source: "org" })).toEqual({ kind: "clear" });
  });

  it("picks the row's face", () => {
    expect(roleView({ ...base, scope: org, chosenId: "gone" })).toBe("missing");
    expect(roleView({ ...base, scope: org })).toBe("inherited");
    expect(roleView({ ...base, scope: org, overriding: true })).toBe("edit");
    expect(roleView({ ...base, scope: org, chosenId: "m2", modelExists: true })).toBe("edit");
    expect(roleView({ ...base, scope: platform, role: "decision" })).toBe("fast");
    expect(roleView({ ...base, scope: platform, role: "decision", ownMode: true })).toBe("edit");
    expect(roleView({ ...base, scope: platform })).toBe("edit");
  });

  it("starts on the chosen model's provider, else the inherited one", () => {
    const models = [
      { id: "a", providerId: "p1" },
      { id: "b", providerId: "p2" },
    ];
    expect(initialProviderId(models, "a", "b")).toBe("p1");
    expect(initialProviderId(models, null, "b")).toBe("p2");
    expect(initialProviderId(models, null, null)).toBeUndefined();
  });
});
