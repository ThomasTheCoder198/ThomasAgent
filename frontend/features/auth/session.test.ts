import { describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";

vi.mock("server-only", () => ({}));
const { redirect } = vi.hoisted(() => ({
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));
vi.mock("next/navigation", () => ({ redirect }));
vi.mock("@/lib/api/server", () => ({
  serverFetch: vi.fn(async () => {
    throw new ApiError("AUTH_SESSION_EXPIRED", "expired", 401);
  }),
}));

describe("requireUser", () => {
  it("redirects to login when core says unauthenticated", async () => {
    const { requireUser } = await import("./session");
    await expect(requireUser("/chat")).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith("/login?next=%2Fchat");
  });
});
