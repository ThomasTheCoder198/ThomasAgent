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
const notify = { success: vi.fn(), error: vi.fn(), info: vi.fn(), problem: vi.fn() };
vi.mock("@/lib/useNotify", () => ({ useNotify: () => notify }));
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

const retryButton = () => screen.getByRole("button", { name: messages.kb.retryOrg });

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

  it("offers Retry to a pending org only once it stalled; Retry provisions once, toasts and refreshes", async () => {
    retryOrganization.mockResolvedValue(org({ ragxStatus: "ready" }));
    renderStation({});
    expect(screen.queryByRole("button", { name: messages.kb.retryOrg })).not.toBeInTheDocument();
    act(() => void vi.advanceTimersByTime(FILE_POLL_INTERVAL_MS * ORG_PENDING_MAX_REFRESHES));
    refresh.mockClear();
    await act(async () => void fireEvent.click(retryButton()));
    expect(retryOrganization).toHaveBeenCalledTimes(1);
    expect(retryOrganization).toHaveBeenCalledWith("o1", "en");
    expect(notify.success).toHaveBeenCalledTimes(1);
    expect(refresh).toHaveBeenCalled();
  });

  it("shows a failed org with its code in mono and Retry at once, and no self-refresh", () => {
    renderStation({ ragxStatus: "failed", ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED" });
    expect(screen.getByText("LLM_ROLE_NOT_ASSIGNED").className).toMatch(/tabular/);
    expect(screen.getByText(messages.kb.orgFailed)).toBeInTheDocument();
    expect(retryButton()).toBeInTheDocument();
    expect(screen.queryByTestId("orb")).not.toBeInTheDocument();
    act(() => void vi.advanceTimersByTime(FILE_POLL_INTERVAL_MS * 3));
    expect(refresh).not.toHaveBeenCalled();
  });

  it("does not toast success when provisioning answers failed again, and still refreshes", async () => {
    retryOrganization.mockResolvedValue(org({ ragxStatus: "failed", ragxErrorCode: "X" }));
    renderStation({ ragxStatus: "failed", ragxErrorCode: "X" });
    await act(async () => void fireEvent.click(retryButton()));
    expect(notify.success).not.toHaveBeenCalled();
    expect(refresh).toHaveBeenCalled();
  });

  it("toasts the failure, which carries the request id for 5xx, when Retry fails", async () => {
    const { ApiError } = await import("@/lib/api/client");
    const error = new ApiError("RAGX_UNAVAILABLE" as never, "Down", 503, undefined, "req_abcdefgh");
    retryOrganization.mockRejectedValue(error);
    renderStation({ ragxStatus: "failed", ragxErrorCode: "RAGX_UNAVAILABLE" });
    await act(async () => void fireEvent.click(retryButton()));
    expect(notify.error).toHaveBeenCalledWith(error);
    expect(refresh).not.toHaveBeenCalled();
  });
});
