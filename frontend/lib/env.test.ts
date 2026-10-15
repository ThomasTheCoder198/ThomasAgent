import { afterEach, describe, expect, it, vi } from "vitest";

async function loadEnv() {
  vi.resetModules();
  return (await import("./env")).env;
}

describe("env", () => {
  afterEach(() => vi.unstubAllEnvs());
  it("defaults the core URL", async () => {
    vi.stubEnv("WEB_CORE_URL", undefined);
    expect((await loadEnv()).WEB_CORE_URL).toBe("http://localhost:8080");
  });
  it("rejects a core URL that is not a URL", async () => {
    vi.stubEnv("WEB_CORE_URL", "core");
    await expect(loadEnv()).rejects.toThrow();
  });
  // Owner decision P14 (2026-10-14): core's UI never links to the ragx admin UI.
  it("does not read a ragx UI link", async () => {
    vi.stubEnv("WEB_RAGX_UI_URL", "http://localhost:8090/ui");
    expect(Object.keys(await loadEnv())).toEqual(["WEB_CORE_URL"]);
  });
});
