import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch, apiFetchPage, describeApiError, mutationHeaders } from "./client";
import { REQUEST_ID_PATTERN } from "./request-id";

function mockFetch(response: Response | Error) {
  const fn = vi.fn<typeof fetch>(async () => {
    if (response instanceof Error) throw response;
    return response.clone();
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
    mockFetch(json(409, { error: { code: "CONFLICT", message: "Xung đột", requestId: "req_abcdefgh" } }));
    const err = await apiFetch("/api/v1/thing").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      code: "CONFLICT",
      status: 409,
      message: "Xung đột",
      requestId: "req_abcdefgh",
    });
  });

  it("falls back to the response header for the request id", async () => {
    mockFetch(
      new Response(JSON.stringify({ error: { code: "CONFLICT", message: "Xung đột" } }), {
        status: 409,
        headers: { "Content-Type": "application/json", "X-Request-Id": "req_fromheader" },
      }),
    );
    await expect(apiFetch("/api/v1/thing")).rejects.toMatchObject({ requestId: "req_fromheader" });
  });

  it("sends a new valid X-Request-Id on every call", async () => {
    const fn = mockFetch(json(200, { data: [], meta: { requestId: "r" } }));
    await apiFetch("/api/v1/providers");
    await apiFetch("/api/v1/providers", { method: "POST", body: "{}" });
    const ids = fn.mock.calls.map((call) => new Headers(call[1]?.headers).get("X-Request-Id"));
    expect(ids).toHaveLength(2);
    for (const id of ids) expect(REQUEST_ID_PATTERN.test(id ?? "")).toBe(true);
    expect(ids[0]).not.toBe(ids[1]);
  });

  it("gives the chat transport a fresh request id per request", () => {
    const first = mutationHeaders("vi")["X-Request-Id"];
    const second = mutationHeaders("vi")["X-Request-Id"];
    expect(REQUEST_ID_PATTERN.test(first ?? "")).toBe(true);
    expect(first).not.toBe(second);
  });

  it("describes errors with the request id", () => {
    const label = (id: string) => `(Mã yêu cầu: ${id})`;
    const withId = new ApiError("CONFLICT" as never, "Xung đột", 409, undefined, "req_abcdefgh");
    expect(describeApiError(withId, "fallback", label)).toBe("Xung đột (Mã yêu cầu: req_abcdefgh)");
    expect(describeApiError(new ApiError("CONFLICT" as never, "Xung đột", 409), "fallback", label)).toBe(
      "Xung đột",
    );
    expect(describeApiError(new Error("x"), "fallback", label)).toBe("fallback");
  });

  it.each(["GET", "HEAD", "OPTIONS"])("replaces supplied request IDs on each %s call", async (method) => {
    const fn = mockFetch(json(200, { data: [] }));
    for (let attempt = 0; attempt < 2; attempt++) {
      await apiFetch("/api/v1/providers", { method, headers: { "X-Request-Id": "req_suppliedid" } });
    }
    const ids = fn.mock.calls.map((call) => new Headers(call[1]?.headers).get("X-Request-Id"));
    for (const id of ids) {
      expect(REQUEST_ID_PATTERN.test(id ?? "")).toBe(true);
      expect(id).not.toBe("req_suppliedid");
    }
    expect(ids[0]).not.toBe(ids[1]);
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

  it("resolves undefined for 204 No Content (ragx deletes through the proxy)", async () => {
    mockFetch(new Response(null, { status: 204 }));
    await expect(
      apiFetch<void>("/api/v1/orgs/o/apps/ragx/knowledge-bases/k", { method: "DELETE" }),
    ).resolves.toBeUndefined();
  });
});

describe("apiFetchPage", () => {
  it("returns the data with the page cursor from meta.page", async () => {
    mockFetch(
      json(200, { data: [{ id: "f1" }], meta: { requestId: "r", page: { cursor: "c2", limit: 20 } } }),
    );
    await expect(apiFetchPage<{ id: string }[]>("/api/v1/files")).resolves.toEqual({
      data: [{ id: "f1" }],
      page: { cursor: "c2", limit: 20 },
    });
  });

  it("throws ApiError like apiFetch on errors", async () => {
    mockFetch(json(404, { error: { code: "KB_NOT_FOUND", message: "Không có", requestId: "req_abcdefgh" } }));
    await expect(apiFetchPage("/api/v1/files")).rejects.toMatchObject({
      code: "KB_NOT_FOUND",
      status: 404,
      requestId: "req_abcdefgh",
    });
    mockFetch(new TypeError("Failed to fetch"));
    await expect(apiFetchPage("/api/v1/files")).rejects.toMatchObject({ code: "CLIENT_NETWORK_ERROR" });
  });
});
