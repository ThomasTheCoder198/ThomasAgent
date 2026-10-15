import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { ThemeProvider } from "next-themes";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Toaster } from "@/components/ui/Sonner";
import messages from "@/messages/en.json";

import { UserMenu } from "./UserMenu";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));
vi.mock("@/features/preferences/actions", () => ({ setLocale: vi.fn() }));

afterEach(() => {
  act(() => void toast.dismiss());
  vi.unstubAllGlobals();
  replace.mockReset();
});

function renderMenu() {
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <ThemeProvider attribute="data-theme" defaultTheme="light">
        <UserMenu />
        <Toaster />
      </ThemeProvider>
    </NextIntlClientProvider>,
  );
}

const openMenu = () => userEvent.click(screen.getByRole("button", { name: messages.shell.settings }));
const reply = (status: number, body: unknown) =>
  vi.fn(async () => new Response(JSON.stringify(body), { status }));

describe("UserMenu", () => {
  it("holds theme, language and sign out", async () => {
    renderMenu();
    await openMenu();
    expect(screen.getAllByRole("menuitemradio")).toHaveLength(5);
    expect(screen.getByRole("menuitem", { name: messages.auth.logout })).toBeInTheDocument();
  });

  it("signs out and goes to the sign-in page", async () => {
    vi.stubGlobal("fetch", reply(200, { data: { status: "ok" } }));
    renderMenu();
    await openMenu();
    await userEvent.click(screen.getByRole("menuitem", { name: messages.auth.logout }));
    await vi.waitFor(() => expect(replace).toHaveBeenCalledWith("/login"));
  });

  it("stays put and toasts when core fails to end the session", async () => {
    vi.stubGlobal("fetch", reply(503, { error: { code: "RAGX_UNAVAILABLE", message: "x" } }));
    renderMenu();
    await openMenu();
    await userEvent.click(screen.getByRole("menuitem", { name: messages.auth.logout }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it("treats an already-expired session as signed out", async () => {
    vi.stubGlobal("fetch", reply(401, { error: { code: "UNAUTHENTICATED", message: "x" } }));
    renderMenu();
    await openMenu();
    await userEvent.click(screen.getByRole("menuitem", { name: messages.auth.logout }));
    await vi.waitFor(() => expect(replace).toHaveBeenCalledWith("/login"));
  });
});
