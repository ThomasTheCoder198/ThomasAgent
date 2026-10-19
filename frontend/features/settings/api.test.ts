import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";

import { saveRoles, testModel } from "./api";

const calls: Array<{ url: string; method: string; body?: string }> = [];

function core(...replies: Array<{ status: number; body?: unknown }>) {
  const queue = [...replies];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: init.method ?? "GET", body: init.body as string | undefined });
      const reply = queue.shift() ?? { status: 200, body: { data: {}, meta: {} } };
      return new Response(reply.status === 204 ? null : JSON.stringify(reply.body), { status: reply.status });
    }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  calls.length = 0;
});

describe("settings api", () => {
  it("saves role changes in order", async () => {
    core({ status: 200, body: { data: { role: "chat.fast", modelId: "m1" }, meta: {} } }, { status: 204 });
    await saveRoles(
      [
        { method: "PUT", path: "/api/v1/model-roles/chat.fast", body: { modelId: "m1" } },
        { method: "DELETE", path: "/api/v1/model-roles/decision" },
      ],
      "vi",
    );
    expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual([
      "PUT /api/v1/model-roles/chat.fast",
      "DELETE /api/v1/model-roles/decision",
    ]);
    expect(JSON.parse(calls[0]?.body ?? "{}")).toEqual({ modelId: "m1" });
    expect(calls[1]?.body).toBeUndefined();
  });

  it("stops at the first failure and rejects with the ApiError", async () => {
    core({
      status: 422,
      body: { error: { code: "LLM_MODEL_CAPABILITY_MISMATCH", message: "x", requestId: "r1" } },
    });
    const run = saveRoles(
      [
        { method: "PUT", path: "/api/v1/model-roles/chat.default", body: { modelId: "m1" } },
        { method: "DELETE", path: "/api/v1/model-roles/decision" },
      ],
      "vi",
    );
    await expect(run).rejects.toBeInstanceOf(ApiError);
    expect(calls).toHaveLength(1);
  });

  it("posts a model test to the model's test path", async () => {
    core({ status: 200, body: { data: { ok: true, latencyMs: 412 }, meta: {} } });
    const result = await testModel("m1", "vi");
    expect(result).toEqual({ ok: true, latencyMs: 412 });
    expect(calls[0]).toMatchObject({ url: "/api/v1/models/m1/test", method: "POST" });
  });
});
