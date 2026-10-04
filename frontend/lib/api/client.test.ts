import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch } from "./client";

function mockFetch(response: Response | Error) {
  const fn = vi.fn<typeof fetch>(async () => {
    if (response instanceof Error) throw response;
    return response;
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
  document.cookie = "thomas_csrf=; max-age=0";
});

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

describe("apiFetch", () => {
  it("unwraps data", async () => {
    mockFetch(json(200, { data: { hello: "world" }, meta: { requestId: "req_1" } }));
    await expect(apiFetch<{ hello: string }>("/api/v1/thing")).resolves.toEqual({ hello: "world" });
  });

  it("throws ApiError from the error response", async () => {
    mockFetch(json(409, { error: { code: "CONFLICT", message: "Xung đột", traceId: "t1" } }));
    const err = await apiFetch("/api/v1/thing").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ code: "CONFLICT", status: 409, message: "Xung đột", traceId: "t1" });
  });

  it("attaches the CSRF header on mutations", async () => {
    document.cookie = "thomas_csrf=abc123";
    const fn = mockFetch(json(201, { data: {}, meta: { requestId: "r" } }));
    await apiFetch("/api/v1/providers", { method: "POST", body: "{}" });
    const headers = new Headers(fn.mock.calls[0]?.[1]?.headers);
    expect(headers.get("X-CSRF-Token")).toBe("abc123");
    expect(headers.get("Content-Type")).toBe("application/json");
  });

  it("does not send the CSRF header on GET", async () => {
    document.cookie = "thomas_csrf=abc123";
    const fn = mockFetch(json(200, { data: [], meta: { requestId: "r" } }));
    await apiFetch("/api/v1/providers");
    expect(new Headers(fn.mock.calls[0]?.[1]?.headers).get("X-CSRF-Token")).toBeNull();
  });

  it("maps network failures and non-JSON bodies", async () => {
    mockFetch(new TypeError("Failed to fetch"));
    await expect(apiFetch("/api/v1/x")).rejects.toMatchObject({ code: "CLIENT_NETWORK_ERROR" });
    mockFetch(new Response("<html>bad gateway</html>", { status: 502 }));
    await expect(apiFetch("/api/v1/x", { locale: "en" })).rejects.toMatchObject({
      code: "CLIENT_NETWORK_ERROR",
      message: "Could not reach the server.",
    });
  });
});
