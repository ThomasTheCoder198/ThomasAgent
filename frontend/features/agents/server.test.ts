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
  it("lets only the agent list fall back to empty on 404", async () => {
    core({ "/api/v1/agents": fail(404, "NOT_FOUND", "x") });
    const { listAgents } = await import("./server");
    await expect(listAgents()).resolves.toEqual([]);
  });

  it("surfaces knowledge-base, model and role failures instead of treating them as empty", async () => {
    core({
      "/api/v1/agents/a1": ok({ id: "a1", kbId: "kb-1" }),
      "/api/v1/knowledge-bases": fail(404, "NOT_FOUND", "Không tìm thấy"),
      "/api/v1/models": fail(500, "INTERNAL_ERROR", "Lỗi máy chủ"),
      "/api/v1/model-roles": ok([]),
    });
    const { getAgentPage } = await import("./server");
    const page = await getAgentPage("a1");
    expect(page?.knowledgeBases).toEqual({ ok: false, message: "Không tìm thấy" });
    expect(page?.models).toEqual({ ok: false, message: "Lỗi máy chủ" });
    expect(page?.roles).toEqual({ ok: true, data: [] });
  });

  it("returns null only when the agent itself is missing", async () => {
    core({
      "/api/v1/agents/a1": fail(404, "NOT_FOUND", "x"),
      "/api/v1/knowledge-bases": ok([]),
      "/api/v1/models": ok([]),
      "/api/v1/model-roles": ok([]),
    });
    const { getAgentPage } = await import("./server");
    await expect(getAgentPage("a1")).resolves.toBeNull();
  });
});
