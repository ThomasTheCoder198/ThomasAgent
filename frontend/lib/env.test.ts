import { afterEach, describe, expect, it, vi } from "vitest";

async function loadEnv() {
  vi.resetModules();
  return (await import("./env")).env;
}

describe("env", () => {
  afterEach(() => vi.unstubAllEnvs());
  it("reads the optional ragx UI link", async () => {
    vi.stubEnv("WEB_RAGX_UI_URL", "http://localhost:8090/ui");
    expect((await loadEnv()).WEB_RAGX_UI_URL).toBe("http://localhost:8090/ui");
  });
  it("treats an empty value as unset", async () => {
    vi.stubEnv("WEB_RAGX_UI_URL", "");
    expect((await loadEnv()).WEB_RAGX_UI_URL).toBeUndefined();
  });
  it("rejects a value that is not a URL", async () => {
    vi.stubEnv("WEB_RAGX_UI_URL", "ragx-ui");
    await expect(loadEnv()).rejects.toThrow();
  });
});
