import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { ThemeProvider } from "next-themes";
import { beforeEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/vi.json";

import { PreferencesMenu } from "./PreferencesMenu";

const setLocale = vi.hoisted(() => vi.fn());
vi.mock("./actions", () => ({ setLocale }));

function renderMenu() {
  render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <ThemeProvider attribute="data-theme" defaultTheme="system" enableSystem>
        <PreferencesMenu />
      </ThemeProvider>
    </NextIntlClientProvider>,
  );
}

async function openMenu() {
  await userEvent.click(screen.getByRole("button", { name: messages.preferences.menu }));
}

beforeEach(() => vi.clearAllMocks());

describe("PreferencesMenu", () => {
  it("offers light, dark and system as radio items and switches the html theme", async () => {
    renderMenu();
    await openMenu();
    expect(screen.getAllByRole("menuitemradio").map((item) => item.textContent)).toEqual([
      "Sáng",
      "Tối",
      "Theo hệ thống",
      "Tiếng Việt",
      "English",
    ]);
    await userEvent.click(screen.getByRole("menuitemradio", { name: "Tối" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    await openMenu();
    expect(screen.getByRole("menuitemradio", { name: "Tối" })).toBeChecked();
    await userEvent.click(screen.getByRole("menuitemradio", { name: "Sáng" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("marks the current language and saves the other one", async () => {
    renderMenu();
    await openMenu();
    expect(screen.getByRole("menuitemradio", { name: "Tiếng Việt" })).toBeChecked();
    await userEvent.click(screen.getByRole("menuitemradio", { name: "English" }));
    expect(setLocale).toHaveBeenCalledWith("en");
  });
});
