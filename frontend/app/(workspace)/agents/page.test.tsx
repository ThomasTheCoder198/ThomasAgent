import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new Error("not found");
  }),
}));

vi.mock("next-intl/server", () => ({ getTranslations: async () => (key: string) => key }));
vi.mock("@/features/orgs/server", () => ({
  getCurrentOrg: async () => ({ org: { id: "org-a" }, failed: false }),
}));
vi.mock("@/features/agents/server", () => ({ listAgents: async () => ({ ok: true, data: [] }) }));
vi.mock("@/features/chat/server", () => ({ getModelChoice: async () => ({ models: [] }) }));

const listKnowledgeBases = vi.fn();
vi.mock("@/features/knowledge-bases/server", () => ({ listKnowledgeBases: () => listKnowledgeBases() }));

import { notFound } from "next/navigation";

import { LoadFailure, ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";

import AgentsPage from "./page";

describe("AgentsPage", () => {
  it("shows a retryable error with the request id when the KB list fails", async () => {
    listKnowledgeBases.mockResolvedValue({
      ok: false,
      serviceDown: true,
      message: "KB service unavailable",
      requestId: "req_kb-failure",
    });

    const page = await AgentsPage();
    expect(page.type).toBe(ServiceUnavailable);
    expect(page.props.requestId).toBe("req_kb-failure");
  });

  it("shows not-found when the KB list returns 404", async () => {
    listKnowledgeBases.mockResolvedValue({
      ok: false,
      notFound: true,
      serviceDown: false,
      message: "KB not found",
    });

    await expect(AgentsPage()).rejects.toThrow("not found");
    expect(notFound).toHaveBeenCalledOnce();
  });

  it("shows a catalog refusal with retry and no Create action for a KB 4xx", async () => {
    listKnowledgeBases.mockResolvedValue({
      ok: false,
      notFound: false,
      serviceDown: false,
      message: "KB access refused",
      requestId: "req_kb-refusal",
    });

    const page = await AgentsPage();
    const [header, content] = page.props.children;
    expect(header.props.actions).toBeUndefined();
    expect(content.props.children.type).toBe(LoadFailure);
    expect(content.props.children.props).toEqual({
      message: "KB access refused",
      requestId: "req_kb-refusal",
    });
  });
});
