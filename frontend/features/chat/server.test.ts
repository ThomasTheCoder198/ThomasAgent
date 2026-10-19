import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "" }),
  headers: async () => new Headers(),
}));

const seen: string[] = [];
function core(routes: Record<string, { status: number; body: unknown }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const parsed = new URL(url);
      const path = parsed.pathname + parsed.search;
      seen.push(path);
      const reply = routes[path] ?? { status: 404, body: { error: { code: "NOT_FOUND", message: "x" } } };
      return new Response(JSON.stringify(reply.body), { status: reply.status });
    }),
  );
}
const ok = (data: unknown) => ({ status: 200, body: { data, meta: { requestId: "r" } } });

afterEach(() => {
  vi.unstubAllGlobals();
  seen.length = 0;
});

describe("chat server loaders", () => {
  it("read recents and a conversation under the org", async () => {
    core({
      "/api/v1/orgs/o1/conversations": ok([
        { id: "c1", title: "T", agentId: "a1", lines: ["kb"], updatedAt: "x" },
      ]),
      "/api/v1/orgs/o1/conversations/c1": ok({
        id: "c1",
        title: "T",
        agentId: "a1",
        scope: {},
        messages: [],
      }),
    });
    const { getConversation, listRecents } = await import("./server");
    expect(await listRecents("o1")).toHaveLength(1);
    expect((await getConversation("o1", "c1"))?.id).toBe("c1");
    expect(seen).toEqual(["/api/v1/orgs/o1/conversations", "/api/v1/orgs/o1/conversations/c1"]);
  });

  it("answers null for a conversation the org does not have", async () => {
    core({
      "/api/v1/orgs/o1/conversations/other": {
        status: 404,
        body: { error: { code: "CONVERSATION_NOT_FOUND", message: "x" } },
      },
    });
    const { getConversation } = await import("./server");
    await expect(getConversation("o1", "other")).resolves.toBeNull();
  });

  it("uses the org's effective chat.default role as the default model", async () => {
    core({
      "/api/v1/models?capability=chat": ok([
        { id: "m1", displayName: "One", capabilities: ["chat"], contextWindow: 1 },
        { id: "m2", displayName: "Two", capabilities: ["chat"], contextWindow: 1 },
      ]),
      "/api/v1/orgs/o1/model-roles": ok([
        { role: "chat.default", modelId: null, effectiveModelId: "m2", source: "platform" },
      ]),
    });
    const { getModelChoice } = await import("./server");
    expect((await getModelChoice("o1")).defaultModelId).toBe("m2");
  });

  it("leaves the default unset when chat.default is not assigned, never the first listed model", async () => {
    core({
      "/api/v1/models?capability=chat": ok([
        { id: "m1", displayName: "One", capabilities: ["chat"], contextWindow: 1 },
      ]),
      "/api/v1/orgs/o1/model-roles": ok([
        { role: "chat.default", modelId: null, effectiveModelId: null, source: "none" },
      ]),
    });
    const { getModelChoice } = await import("./server");
    expect((await getModelChoice("o1")).defaultModelId).toBeUndefined();
  });
});
