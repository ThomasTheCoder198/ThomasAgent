import { describe, expect, it } from "vitest";

import type { OrgModelRole } from "@/lib/core/types";

import { orgCurrent, orgOverrideCount, platformCurrent } from "./scope-roles";

const orgRole = (
  role: OrgModelRole["role"],
  modelId: string | null,
  effective: string | null,
  source: OrgModelRole["source"],
) => ({ role, modelId, effectiveModelId: effective, source }) satisfies OrgModelRole;

describe("scope-roles", () => {
  it("reads a platform role, and none when it is unassigned", () => {
    const roles = [{ role: "chat.fast" as const, modelId: "m1" }];
    expect(platformCurrent(roles, "chat.fast")).toEqual({ modelId: "m1", source: "platform" });
    expect(platformCurrent(roles, "vision")).toEqual({ modelId: null, source: "none" });
  });

  it("reads an org role: its own override, else what it inherits", () => {
    const roles = [
      orgRole("chat.default", "m2", "m2", "org"),
      orgRole("chat.fast", null, "m1", "platform"),
      orgRole("decision", null, "m1", "fallback"),
      orgRole("vision", null, null, "none"),
    ];
    expect(orgCurrent(roles, "chat.default")).toEqual({ modelId: "m2", source: "org" });
    expect(orgCurrent(roles, "chat.fast")).toEqual({ modelId: "m1", source: "platform" });
    expect(orgCurrent(roles, "decision")).toEqual({ modelId: "m1", source: "fallback" });
    expect(orgCurrent(roles, "vision")).toEqual({ modelId: null, source: "none" });
  });

  it("counts the roles an org overrides", () => {
    const roles = [orgRole("chat.default", "m2", "m2", "org"), orgRole("chat.fast", null, "m1", "platform")];
    expect(orgOverrideCount(roles)).toBe(1);
    expect(orgOverrideCount(undefined)).toBe(0);
  });
});
