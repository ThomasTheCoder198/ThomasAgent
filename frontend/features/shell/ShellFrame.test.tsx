import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { ShellFrame } from "./ShellFrame";

const nav = vi.hoisted(() => ({ pathname: "/tools", belowMd: true }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.pathname }));
vi.mock("@/hooks/useMediaQuery", () => ({ useIsBelowMd: () => nav.belowMd }));

const labels = {
  openMenu: "Open menu",
  closeMenu: "Close menu",
  collapse: "Collapse sidebar",
  expand: "Expand sidebar",
  drawerTitle: "Main navigation",
};

function frame() {
  return (
    <NextIntlClientProvider locale="en" messages={messages}>
      <ShellFrame
        initialCollapsed={false}
        labels={labels}
        sidebar={
          <aside>
            <a href="#chat">Recent chat</a>
          </aside>
        }
      >
        <p>Page</p>
      </ShellFrame>
    </NextIntlClientProvider>
  );
}

beforeEach(() => {
  nav.pathname = "/tools";
  nav.belowMd = true;
});

describe("ShellFrame drawer", () => {
  it("opens the sidebar in a named dialog from the top bar and closes it with Escape", async () => {
    render(frame());
    expect(screen.queryByRole("dialog")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: labels.openMenu }));
    const drawer = await screen.findByRole("dialog", { name: labels.drawerTitle });
    expect(drawer).toHaveTextContent("Recent chat");
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: labels.openMenu })).toHaveFocus();
  });

  it("closes when a link inside it is followed", async () => {
    render(frame());
    await userEvent.click(screen.getByRole("button", { name: labels.openMenu }));
    const drawer = await screen.findByRole("dialog");
    await userEvent.click(drawer.querySelector("a") as HTMLElement);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("leaves the menu button to pages with their own header", () => {
    nav.pathname = "/chat/c1";
    render(frame());
    expect(screen.queryByRole("button", { name: labels.openMenu })).toBeNull();
  });
});
