import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";

import { SESSION_COOKIE } from "@/features/auth/paths";
import { CURRENT_ORG_COOKIE } from "@/features/orgs/contract";

import { proxy } from "./proxy";

describe("proxy", () => {
  it("remembers the org a page under /orgs/{id} shows", () => {
    const request = new NextRequest("http://localhost/orgs/3f2b/knowledge-bases", {
      headers: { cookie: `${SESSION_COOKIE}=s` },
    });
    expect(proxy(request).cookies.get(CURRENT_ORG_COOKIE)?.value).toBe("3f2b");
  });
  it("leaves the cookie alone elsewhere", () => {
    const request = new NextRequest("http://localhost/agents", {
      headers: { cookie: `${SESSION_COOKIE}=s` },
    });
    expect(proxy(request).cookies.get(CURRENT_ORG_COOKIE)).toBeUndefined();
  });
});
