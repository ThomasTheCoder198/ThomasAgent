import { describe, expect, it } from "vitest";

import { planSave, type RoleDraft } from "./useRoleDraft";

describe("planSave", () => {
  it("turns a platform draft into PUTs and DELETEs on /model-roles", () => {
    const draft: RoleDraft = { "chat.default": { kind: "set", modelId: "m1" }, decision: { kind: "clear" } };
    expect(planSave({ scope: "platform" }, draft)).toEqual([
      { method: "PUT", path: "/api/v1/model-roles/chat.default", body: { modelId: "m1" } },
      { method: "DELETE", path: "/api/v1/model-roles/decision" },
    ]);
  });

  it("turns an org draft into org-scoped requests", () => {
    const draft: RoleDraft = { "chat.fast": { kind: "set", modelId: "m2" }, vision: { kind: "clear" } };
    expect(planSave({ scope: "org", orgId: "o1" }, draft)).toEqual([
      { method: "PUT", path: "/api/v1/orgs/o1/model-roles/chat.fast", body: { modelId: "m2" } },
      { method: "DELETE", path: "/api/v1/orgs/o1/model-roles/vision" },
    ]);
  });

  it("sends nothing for an empty draft", () => {
    expect(planSave({ scope: "platform" }, {})).toEqual([]);
  });

  it("orders requests by role display order, whatever order the edits were made in", () => {
    const draft: RoleDraft = { vision: { kind: "clear" }, "chat.default": { kind: "set", modelId: "m1" } };
    expect(planSave({ scope: "platform" }, draft).map((r) => r.path)).toEqual([
      "/api/v1/model-roles/chat.default",
      "/api/v1/model-roles/vision",
    ]);
  });
});
