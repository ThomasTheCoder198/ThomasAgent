import { describe, expect, it } from "vitest";

import { ORG_NAME_MAX_LENGTH, orgHome, validateOrgName } from "./contract";

describe("org name validation mirrors core", () => {
  it("trims, bounds and rejects control characters", () => {
    expect(validateOrgName("   ")).toBe("required");
    expect(validateOrgName("ư".repeat(ORG_NAME_MAX_LENGTH + 1))).toBe("tooLong");
    expect(validateOrgName("Acme\u0000")).toBe("invalid");
    expect(validateOrgName("  Công ty Acme ")).toBeNull();
  });
  it("links to the org's Knowledge Bases", () => {
    expect(orgHome("a b")).toBe("/orgs/a%20b/knowledge-bases");
  });
});
