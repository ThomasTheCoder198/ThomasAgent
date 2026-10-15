import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import type { Organization } from "./contract";
import { OrgPlate } from "./OrgPlate";

const nav = vi.hoisted(() => ({ pathname: "/chat" }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
  usePathname: () => nav.pathname,
}));
vi.mock("./api", () => ({
  createOrganization: vi.fn(),
  retryOrganization: vi.fn(),
  updateOrganization: vi.fn(),
}));

const demo: Organization = {
  id: "o1",
  name: "Công ty Demo",
  externalParsing: true,
  ragxStatus: "ready",
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-10T00:00:00Z",
  updatedAt: "2026-10-10T00:00:00Z",
};

function plate() {
  return (
    <NextIntlClientProvider locale="en" messages={messages}>
      <OrgPlate orgs={[demo]} current={demo} />
    </NextIntlClientProvider>
  );
}

describe("OrgPlate", () => {
  beforeEach(() => {
    nav.pathname = "/chat";
  });

  it("follows the org in the URL when the server-rendered layout is stale", () => {
    const second = { ...demo, id: "o2", name: "Beta" };
    nav.pathname = "/orgs/o2/knowledge-bases";
    render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <OrgPlate orgs={[demo, second]} current={demo} />
      </NextIntlClientProvider>,
    );
    expect(screen.getByRole("button", { name: /Organization: Beta/ })).toBeInTheDocument();
  });

  it("draws the whole first character in the roundel, even outside the BMP", () => {
    const astral = { ...demo, name: "\u{1D56B}elta" };
    const { container, rerender } = render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <OrgPlate orgs={[astral]} current={astral} />
      </NextIntlClientProvider>,
    );
    expect(container.querySelector("span[aria-hidden]")?.textContent).toBe("\u{1D56B}");
    const empty = { ...demo, name: "" };
    rerender(
      <NextIntlClientProvider locale="en" messages={messages}>
        <OrgPlate orgs={[empty]} current={empty} />
      </NextIntlClientProvider>,
    );
    expect(container.querySelector("span[aria-hidden]")?.textContent).toBe("");
  });

  // Owner decision P14 (2026-10-14): the ragx admin UI is separate; core's menu never links to it.
  it("has no link out of core in the org menu", async () => {
    render(plate());
    await userEvent.click(screen.getByRole("button", { name: /Organization: Công ty Demo/ }));
    const items = await screen.findAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      demo.name,
      messages.orgs.settings,
      messages.orgs.new,
    ]);
    expect(document.querySelector('a[target="_blank"]')).toBeNull();
    expect(messages.orgs).not.toHaveProperty("ragxModels");
  });
});
