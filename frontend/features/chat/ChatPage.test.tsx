import { describe, expect, it, vi } from "vitest";

const current = vi.hoisted(() => ({ value: { org: null, orgs: [], failed: false } as unknown }));
vi.mock("next-intl/server", () => ({ getTranslations: async () => (key: string) => key }));
vi.mock("@/features/orgs/server", () => ({ getCurrentOrg: async () => current.value }));
vi.mock("@/features/agents/server", () => ({ listAgents: vi.fn() }));
vi.mock("./server", () => ({ getConversation: vi.fn(), getModelChoice: vi.fn() }));

import { ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { NoOrgState } from "@/features/orgs/NoOrgState";
import { listAgents } from "@/features/agents/server";

import { getConversation, getModelChoice } from "./server";

import { ChatPage } from "./ChatPage";

describe("ChatPage org states", () => {
  it("shows the retryable service error when the org list failed", async () => {
    current.value = { org: null, orgs: [], failed: true };
    expect((await ChatPage({ chatId: "c1" })).type).toBe(ServiceUnavailable);
  });

  it("shows the no-org state only when the user really has no org", async () => {
    current.value = { org: null, orgs: [], failed: false };
    expect((await ChatPage({ chatId: "c1" })).type).toBe(NoOrgState);
  });

  it("shows the Agent API failure and its request id instead of an empty composer", async () => {
    current.value = { org: { id: "org-a" }, orgs: [], failed: false };
    vi.mocked(getConversation).mockResolvedValue(null);
    vi.mocked(getModelChoice).mockResolvedValue({} as Awaited<ReturnType<typeof getModelChoice>>);
    vi.mocked(listAgents).mockResolvedValue({
      ok: false,
      message: "Agent API unavailable",
      requestId: "req_agent-failure",
    });

    const page = await ChatPage({ chatId: "c1" });
    expect(page.type).toBe(ServiceUnavailable);
    expect(page.props.requestId).toBe("req_agent-failure");
  });
});
