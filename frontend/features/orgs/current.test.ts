import { describe, expect, it } from "vitest";

import type { Organization } from "./contract";
import { orgIdFromPath, resolveCurrentOrg } from "./current";

const org = (id: string, ragxStatus: Organization["ragxStatus"] = "ready"): Organization => ({
  id,
  name: id,
  externalParsing: true,
  ragxStatus,
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-08T00:00:00Z",
  updatedAt: "2026-10-08T00:00:00Z",
});

describe("current org", () => {
  const orgs = [org("a"), org("b"), org("c", "pending")];
  it("prefers the org in the path, then the cookie, then the first org", () => {
    expect(resolveCurrentOrg(orgs, "b", "a")?.id).toBe("b");
    expect(resolveCurrentOrg(orgs, null, "c")?.id).toBe("c");
    expect(resolveCurrentOrg(orgs, null, "gone")?.id).toBe("a");
    expect(resolveCurrentOrg([], null, "a")).toBeNull();
  });
  it("never trusts a path or cookie id that is not in the list", () => {
    expect(resolveCurrentOrg(orgs, "zzz", null)?.id).toBe("a");
  });
  it("reads the org id from /orgs/{id}/…", () => {
    expect(orgIdFromPath("/orgs/3f2b/knowledge-bases/x")).toBe("3f2b");
    expect(orgIdFromPath("/orgs/3f2b")).toBe("3f2b");
    expect(orgIdFromPath("/agents")).toBeNull();
  });
  it("returns null for a malformed percent-escape instead of throwing", () => {
    expect(orgIdFromPath("/orgs/%E0/knowledge-bases")).toBeNull();
    expect(orgIdFromPath("/orgs/a%20b")).toBe("a b");
  });
});
