import { afterEach, describe, expect, it, vi } from "vitest";

import { createAgent, deleteAgent } from "./api";

const calls: Array<{ url: string; method: string; body?: string }> = [];

function core(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: init.method ?? "GET", body: init.body as string | undefined });
      return new Response(status === 204 ? null : JSON.stringify(body), { status });
    }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  calls.length = 0;
});

describe("agents api", () => {
  it("creates an agent under the org", async () => {
    core(201, { data: { id: "a1", name: "Pháp chế", kbId: "kb-1" }, meta: { requestId: "r" } });
    const agent = await createAgent("o1", { name: "Pháp chế", kbId: "kb-1" }, "vi");
    expect(agent.id).toBe("a1");
    expect(calls[0]).toMatchObject({ url: "/api/v1/orgs/o1/agents", method: "POST" });
    expect(JSON.parse(calls[0]?.body ?? "{}")).toEqual({ name: "Pháp chế", kbId: "kb-1" });
  });

  it("deletes an agent under the org", async () => {
    core(204, null);
    await deleteAgent("o1", "a1", "vi");
    expect(calls[0]).toMatchObject({ url: "/api/v1/orgs/o1/agents/a1", method: "DELETE" });
  });
});
