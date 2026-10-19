import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

const cookieJar = new Map<string, string>();
let pathname = "/chat";
vi.mock("next/headers", () => ({
  cookies: async () => ({
    toString: () => "",
    get: (name: string) => (cookieJar.has(name) ? { value: cookieJar.get(name) } : undefined),
  }),
  headers: async () => new Headers({ "x-pathname": pathname }),
}));

const org = (id: string) => ({
  id,
  name: id,
  externalParsing: false,
  ragxStatus: "ready",
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-17T00:00:00Z",
  updatedAt: "2026-10-17T00:00:00Z",
});

function core(orgs: unknown[] | null) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      orgs
        ? new Response(JSON.stringify({ data: orgs, meta: { requestId: "r" } }), { status: 200 })
        : new Response(
            JSON.stringify({ error: { code: "INTERNAL_ERROR", message: "x", requestId: "req_down" } }),
            { status: 500 },
          ),
    ),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  cookieJar.clear();
  pathname = "/chat";
});

describe("getCurrentOrg", () => {
  it("prefers the path, then the cookie, then the first org", async () => {
    core([org("a"), org("b"), org("c")]);
    const { getCurrentOrg } = await import("./server");
    cookieJar.set("thomas_org", "b");
    expect((await getCurrentOrg()).org?.id).toBe("b");
    pathname = "/orgs/c/knowledge-bases";
    expect((await getCurrentOrg()).org?.id).toBe("c");
    pathname = "/chat";
    cookieJar.set("thomas_org", "gone");
    expect((await getCurrentOrg()).org?.id).toBe("a");
  });

  it("reports a failed org list instead of pretending there is no org", async () => {
    core(null);
    const { getCurrentOrg } = await import("./server");
    await expect(getCurrentOrg()).resolves.toEqual({
      org: null,
      orgs: [],
      failed: true,
      requestId: "req_down",
    });
  });

  it("reports an unreachable core as failed too (Codex round 2 #3)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("fetch failed");
      }),
    );
    const { getCurrentOrg } = await import("./server");
    await expect(getCurrentOrg()).resolves.toMatchObject({ org: null, orgs: [], failed: true });
  });
});
