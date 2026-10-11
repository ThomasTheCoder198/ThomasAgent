import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { NavLink } from "./NavLink";

const nav = vi.hoisted(() => ({ pathname: "/chat" }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.pathname }));

const scope = { orgIds: ["a", "b"], serverOrgId: "a" };

function link() {
  return (
    <NavLink href="/knowledge-bases" orgScope={scope} match="section" serverPath="/chat">
      Knowledge Base
    </NavLink>
  );
}

describe("NavLink with an org scope", () => {
  beforeEach(() => {
    nav.pathname = "/chat";
  });

  it("points at the server-resolved org away from an org page", () => {
    render(link());
    expect(screen.getByRole("link")).toHaveAttribute("href", "/orgs/a/knowledge-bases");
  });

  it("follows the org in the live URL, which the persistent layout cannot know", () => {
    nav.pathname = "/orgs/b/knowledge-bases/kb1";
    render(link());
    const anchor = screen.getByRole("link");
    expect(anchor).toHaveAttribute("href", "/orgs/b/knowledge-bases");
    expect(anchor).toHaveAttribute("aria-current", "page");
  });

  it("ignores an org id that is not in the operator's list", () => {
    nav.pathname = "/orgs/zzz/knowledge-bases";
    render(link());
    expect(screen.getByRole("link")).toHaveAttribute("href", "/orgs/a/knowledge-bases");
  });

  it("falls back to the plain href when there is no org", () => {
    render(
      <NavLink
        href="/knowledge-bases"
        orgScope={{ orgIds: [], serverOrgId: null }}
        match="section"
        serverPath="/chat"
      >
        Knowledge Base
      </NavLink>,
    );
    expect(screen.getByRole("link")).toHaveAttribute("href", "/knowledge-bases");
  });
});
