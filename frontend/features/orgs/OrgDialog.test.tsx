import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import type { Organization } from "./contract";
import { OrgDialog } from "./OrgDialog";

const router = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
const createOrganization = vi.fn();
const retryOrganization = vi.fn();
const updateOrganization = vi.fn();
vi.mock("./api", () => ({
  createOrganization: (...args: unknown[]) => createOrganization(...args),
  retryOrganization: (...args: unknown[]) => retryOrganization(...args),
  updateOrganization: (...args: unknown[]) => updateOrganization(...args),
}));

function renderDialog() {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <OrgDialog mode="create" open onOpenChange={() => undefined} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("OrgDialog", () => {
  it("validates the name before calling core", async () => {
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.create }));
    expect(await screen.findByText(messages.orgs.errors.nameRequired)).toBeInTheDocument();
    expect(createOrganization).not.toHaveBeenCalled();
  });
  it("sends the trimmed name and the external parsing choice", async () => {
    createOrganization.mockResolvedValue({ id: "o1" });
    renderDialog();
    await userEvent.type(screen.getByLabelText(messages.orgs.name), "  Acme  ");
    await userEvent.click(screen.getByRole("switch", { name: messages.orgs.externalParsing }));
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.create }));
    expect(createOrganization).toHaveBeenCalledWith({ name: "Acme", externalParsing: false }, "en");
  });
  it("opens the new org and refreshes the shell, whose org list is rendered by a layout that stays mounted", async () => {
    createOrganization.mockResolvedValue({ id: "o1" });
    renderDialog();
    await userEvent.type(screen.getByLabelText(messages.orgs.name), "Acme");
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.create }));
    await waitFor(() => expect(router.push).toHaveBeenCalledWith("/orgs/o1/knowledge-bases"));
    expect(router.refresh).toHaveBeenCalledTimes(1);
  });
});

const org = (over: Partial<Organization>): Organization => ({
  id: "o1",
  name: "Beta",
  externalParsing: true,
  ragxStatus: "ready",
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-10T00:00:00Z",
  updatedAt: "2026-10-10T00:00:00Z",
  ...over,
});

function renderEdit(organization: Organization) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <OrgDialog mode="edit" org={organization} open onOpenChange={() => undefined} />
    </NextIntlClientProvider>,
  );
}

describe("OrgDialog settings", () => {
  it.each([
    ["failed", org({ ragxStatus: "failed", ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED", settingsSynced: false })],
    ["stuck pending", org({ ragxStatus: "pending", settingsSynced: false })],
    ["ready but unsynced", org({ settingsSynced: false, ragxErrorCode: "RAGX_UNAVAILABLE" })],
  ])("offers Retry when the org is %s", async (_, organization) => {
    retryOrganization.mockResolvedValue(org({}));
    renderEdit(organization);
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.retry }));
    expect(retryOrganization).toHaveBeenCalledTimes(1);
    expect(retryOrganization).toHaveBeenCalledWith("o1", "en");
  });
  it("shows the error code in mono for a failed org", () => {
    renderEdit(org({ ragxStatus: "failed", ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED", settingsSynced: false }));
    expect(screen.getByText("LLM_ROLE_NOT_ASSIGNED").className).toMatch(/tabular/);
  });
  it("shows the retried result at once", async () => {
    retryOrganization.mockResolvedValue(org({}));
    renderEdit(org({ ragxStatus: "failed", ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED", settingsSynced: false }));
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.retry }));
    expect(await screen.findByText(messages.orgs.status.ready)).toBeInTheDocument();
    expect(screen.queryByText("LLM_ROLE_NOT_ASSIGNED")).toBeNull();
  });
  it("announces the retried state in a status region and keeps focus on it", async () => {
    retryOrganization.mockResolvedValue(org({}));
    renderEdit(org({ ragxStatus: "pending", settingsSynced: false }));
    const region = screen.getByRole("status");
    expect(region).toHaveTextContent(messages.orgs.status.pending);
    await userEvent.click(screen.getByRole("button", { name: messages.orgs.retry }));
    expect(await screen.findByRole("status")).toHaveTextContent(messages.orgs.status.ready);
    expect(screen.queryByRole("button", { name: messages.orgs.retry })).toBeNull();
    expect(screen.getByRole("status")).toHaveFocus();
  });
  it("shows no Retry when ready and synced", () => {
    renderEdit(org({}));
    expect(screen.queryByRole("button", { name: messages.orgs.retry })).toBeNull();
  });
});

describe("OrgDialog settings dirty state", () => {
  const save = () => screen.getByRole("button", { name: messages.common.save });
  const nameField = () => screen.getByLabelText(messages.orgs.name);

  it("keeps Save disabled until the name differs from the saved one, and again after reverting", async () => {
    renderEdit(org({}));
    expect(save()).toBeDisabled();
    await userEvent.type(nameField(), "  ");
    expect(save()).toBeDisabled();
    await userEvent.clear(nameField());
    await userEvent.type(nameField(), "Gamma");
    expect(save()).toBeEnabled();
    await userEvent.clear(nameField());
    await userEvent.type(nameField(), "Beta");
    expect(save()).toBeDisabled();
  });
  it("enables Save when external parsing is toggled and disables it when toggled back", async () => {
    renderEdit(org({}));
    const toggle = screen.getByRole("switch", { name: messages.orgs.externalParsing });
    await userEvent.click(toggle);
    expect(save()).toBeEnabled();
    await userEvent.click(toggle);
    expect(save()).toBeDisabled();
  });
  it("cannot send an unchanged form, and sends a changed one", async () => {
    updateOrganization.mockResolvedValue(org({ name: "Gamma" }));
    renderEdit(org({}));
    await userEvent.type(nameField(), "{Enter}");
    expect(updateOrganization).not.toHaveBeenCalled();
    await userEvent.clear(nameField());
    await userEvent.type(nameField(), "Gamma");
    await userEvent.click(save());
    expect(updateOrganization).toHaveBeenCalledWith("o1", { name: "Gamma", externalParsing: true }, "en");
  });
});
