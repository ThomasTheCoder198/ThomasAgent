import { act, fireEvent, render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "@/features/orgs/contract";
import messages from "@/messages/en.json";

import { FILE_POLL_INTERVAL_MS, ORG_PENDING_MAX_REFRESHES } from "../contract";
import { OrgStatusStation } from "./OrgStatusStation";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("thinking-orbs", () => ({ ThinkingOrb: () => <span data-testid="orb" /> }));
const retryOrganization = vi.fn();
vi.mock("@/features/orgs/api", () => ({
  retryOrganization: (...args: unknown[]) => retryOrganization(...args),
}));

const org = (over: Partial<Organization>): Organization => ({
  id: "o1",
  name: "Beta",
  externalParsing: true,
  ragxStatus: "pending",
  ragxErrorCode: null,
  settingsSynced: false,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-10T08:00:00Z",
  updatedAt: "2026-10-10T08:00:00Z",
  ...over,
});

function renderStation(over: Partial<Organization>) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <OrgStatusStation org={org(over)} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

describe("OrgStatusStation", () => {
  it("refreshes a pending org a bounded number of times, then stops and shows the stalled copy", () => {
    renderStation({});
    expect(screen.getByTestId("orb")).toBeInTheDocument();
    expect(screen.getByText(messages.kb.orgPending)).toBeInTheDocument();
    act(() => void vi.advanceTimersByTime(FILE_POLL_INTERVAL_MS * (ORG_PENDING_MAX_REFRESHES + 5)));
    expect(refresh).toHaveBeenCalledTimes(ORG_PENDING_MAX_REFRESHES);
    expect(screen.queryByTestId("orb")).not.toBeInTheDocument();
    expect(screen.getByText(messages.kb.orgPendingStalled)).toBeInTheDocument();
  });

  it("always offers Retry, which provisions once and refreshes", async () => {
    retryOrganization.mockResolvedValue(org({ ragxStatus: "ready" }));
    renderStation({});
    await act(async () => void fireEvent.click(screen.getByRole("button", { name: messages.kb.retryOrg })));
    expect(retryOrganization).toHaveBeenCalledTimes(1);
    expect(retryOrganization).toHaveBeenCalledWith("o1", "en");
    expect(refresh).toHaveBeenCalled();
  });

  it("shows a failed org in red with its code and Retry, and no self-refresh", () => {
    renderStation({ ragxStatus: "failed", ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED" });
    expect(screen.getByText("LLM_ROLE_NOT_ASSIGNED")).toBeInTheDocument();
    expect(screen.getByText(messages.kb.orgFailed)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.retryOrg })).toBeInTheDocument();
    act(() => void vi.advanceTimersByTime(FILE_POLL_INTERVAL_MS * 3));
    expect(refresh).not.toHaveBeenCalled();
  });

  it("shows the request id when Retry fails", async () => {
    const { ApiError } = await import("@/lib/api/client");
    retryOrganization.mockRejectedValue(
      new ApiError("RAGX_UNAVAILABLE" as never, "Down", 503, undefined, "req_abcdefgh"),
    );
    renderStation({ ragxStatus: "failed", ragxErrorCode: "RAGX_UNAVAILABLE" });
    await act(async () => void fireEvent.click(screen.getByRole("button", { name: messages.kb.retryOrg })));
    expect(screen.getByRole("alert")).toHaveTextContent("req_abcdefgh");
  });
});
