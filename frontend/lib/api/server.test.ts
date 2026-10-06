import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=browser" }),
  headers: async () => new Headers({ "accept-language": "en-US" }),
}));

import { REQUEST_ID_PATTERN } from "./request-id";
import { serverFetch } from "./server";

afterEach(() => vi.unstubAllGlobals());

describe("serverFetch", () => {
  it.each([
    { "X-Custom": "preserved", "X-Request-Id": "req_suppliedid" },
    new Headers({ "X-Custom": "preserved", "X-Request-Id": "req_suppliedid" }),
    [
      ["X-Custom", "preserved"],
      ["X-Request-Id", "req_suppliedid"],
    ],
  ] satisfies HeadersInit[])(
    "preserves HeadersInit and creates a fresh ID each call: %j",
    async (suppliedHeaders) => {
      const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ data: [] })));
      vi.stubGlobal("fetch", fetchMock);
      await serverFetch("/api/v1/providers", { headers: suppliedHeaders });
      await serverFetch("/api/v1/providers", { headers: suppliedHeaders });
      const ids = fetchMock.mock.calls.map((call) => {
        const headers = new Headers(call[1]?.headers);
        expect(headers.get("X-Custom")).toBe("preserved");
        expect(headers.get("Cookie")).toBe("session=browser");
        expect(headers.get("Accept-Language")).toBe("en");
        expect(call[1]?.cache).toBe("no-store");
        const id = headers.get("X-Request-Id");
        expect(REQUEST_ID_PATTERN.test(id ?? "")).toBe(true);
        expect(id).not.toBe("req_suppliedid");
        return id;
      });
      expect(ids[0]).not.toBe(ids[1]);
    },
  );
});
