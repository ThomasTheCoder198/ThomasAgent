import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "next-themes";
import { describe, expect, it } from "vitest";

import { ThemeSwitcher } from "./ThemeSwitcher";

const labels = { light: "Sáng", dark: "Tối", system: "Theo hệ thống", legend: "Giao diện" };

describe("ThemeSwitcher", () => {
  it("switches the html theme attribute", async () => {
    render(
      <ThemeProvider attribute="data-theme" defaultTheme="system" enableSystem>
        <ThemeSwitcher labels={labels} />
      </ThemeProvider>,
    );
    await userEvent.click(screen.getByRole("radio", { name: "Tối" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    await userEvent.click(screen.getByRole("radio", { name: "Sáng" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
});
