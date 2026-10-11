import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { NavLink } from "../shell/NavLink";
import type { Organization } from "./contract";
import { OrgPlate } from "./OrgPlate";

const nav = vi.hoisted(() => ({ pathname: "/chat" }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
  usePathname: () => nav.pathname,
}));

const org = (id: string, name: string): Organization => ({
  id,
  name,
  externalParsing: false,
  ragxStatus: "ready",
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: "2026-10-10T00:00:00Z",
  updatedAt: "2026-10-10T00:00:00Z",
});
const a = org("a", "Alpha");
const b = org("b", "Beta");

function shell() {
  return (
    <NextIntlClientProvider locale="en" messages={messages}>
      <OrgPlate orgs={[a, b]} current={a} />
      <NavLink
        href="/knowledge-bases"
        orgScope={{ orgIds: ["a", "b"], serverOrgId: "a" }}
        match="section"
        serverPath="/chat"
      >
        Knowledge Base
      </NavLink>
    </NextIntlClientProvider>
  );
}

describe("the plate and the Knowledge Base link share one live org", () => {
  beforeEach(() => {
    nav.pathname = "/chat";
  });

  it("keeps the org switched to when the operator leaves the org pages", () => {
    const view = render(shell());
    expect(screen.getByRole("link")).toHaveAttribute("href", "/orgs/a/knowledge-bases");

    nav.pathname = "/orgs/b/knowledge-bases";
    view.rerender(shell());
    nav.pathname = "/agents";
    view.rerender(shell());

    expect(screen.getByRole("button", { name: /Organization: Beta/ })).toBeInTheDocument();
    expect(screen.getByRole("link")).toHaveAttribute("href", "/orgs/b/knowledge-bases");
  });
});
