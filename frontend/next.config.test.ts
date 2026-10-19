import { describe, expect, it } from "vitest";

import nextConfig, { PROXY_TIMEOUT_MS } from "./next.config";

describe("next.config", () => {
  it("keeps proxied streams open longer than a whole run (spike S8)", () => {
    expect(PROXY_TIMEOUT_MS).toBe(360_000);
    expect(nextConfig.experimental?.proxyTimeout).toBe(PROXY_TIMEOUT_MS);
  });
});
