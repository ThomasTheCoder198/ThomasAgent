import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "" }),
  headers: async () => new Headers(),
}));

type Reply = { status: number; body: unknown };

const ok = (data: unknown): Reply => ({ status: 200, body: { data, meta: { requestId: "r" } } });
const fail = (status: number, code: string, message: string): Reply => ({
  status,
  body: { error: { code, message } },
});

/** Stubs core: every request is answered from the route table by path. */
function core(routes: Record<string, Reply>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const reply = routes[new URL(url).pathname] ?? fail(404, "NOT_FOUND", "missing");
      return new Response(JSON.stringify(reply.body), { status: reply.status });
    }),
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("agents server loaders", () => {
  it("reads the org's agents and reports a failure instead of an empty list", async () => {
    core({ "/api/v1/orgs/o1/agents": fail(500, "INTERNAL_ERROR", "Lỗi máy chủ") });
    const { listAgents } = await import("./server");
    await expect(listAgents("o1")).resolves.toEqual({ ok: false, message: "Lỗi máy chủ" });
  });

  it("lists the org's KBs through the ragx app proxy and carries failures to their tab", async () => {
    core({
      "/api/v1/orgs/o1/agents/a1": ok({ id: "a1", kbId: "kb-1" }),
      "/api/v1/orgs/o1/apps/ragx/knowledge-bases": fail(503, "RAGX_UNAVAILABLE", "Ragx tạm ngưng"),
      "/api/v1/models": fail(500, "INTERNAL_ERROR", "Lỗi máy chủ"),
      "/api/v1/orgs/o1/model-roles": ok([]),
    });
    const { getAgentPage } = await import("./server");
    const page = await getAgentPage("o1", "a1");
    expect(page?.knowledgeBases).toEqual({ ok: false, message: "Ragx tạm ngưng" });
    expect(page?.models).toEqual({ ok: false, message: "Lỗi máy chủ" });
    expect(page?.roles).toEqual({ ok: true, data: [] });
  });

  it("returns null when core says the agent does not exist", async () => {
    core({
      "/api/v1/orgs/o1/agents/a1": fail(404, "AGENT_NOT_FOUND", "x"),
      "/api/v1/orgs/o1/apps/ragx/knowledge-bases": ok([]),
      "/api/v1/models": ok([]),
      "/api/v1/orgs/o1/model-roles": ok([]),
    });
    const { getAgentPage } = await import("./server");
    await expect(getAgentPage("o1", "a1")).resolves.toBeNull();
  });
});
