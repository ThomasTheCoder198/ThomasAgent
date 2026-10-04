import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { Tabs } from "./Tabs";

const items = [
  { id: "a", label: "Một" },
  { id: "b", label: "Hai" },
  { id: "c", label: "Ba" },
] as const;

function Harness() {
  const [value, setValue] = useState<"a" | "b" | "c">("a");
  return <Tabs items={items} value={value} onChange={setValue} idPrefix="t" label="Thử" />;
}

describe("Tabs keyboard", () => {
  it("keeps a roving tabindex: only the selected tab is in the tab order", () => {
    render(<Harness />);
    expect(screen.getByRole("tab", { name: "Một" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: "Hai" })).toHaveAttribute("tabindex", "-1");
  });

  it("moves selection and focus with arrows, wrapping at both ends", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("tab", { name: "Một" }));
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Hai" })).toHaveFocus();
    expect(screen.getByRole("tab", { name: "Hai" })).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(screen.getByRole("tab", { name: "Ba" })).toHaveFocus();
  });

  it("jumps to the first and last tab with Home and End", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("tab", { name: "Một" }));
    await userEvent.keyboard("{End}");
    expect(screen.getByRole("tab", { name: "Ba" })).toHaveFocus();
    expect(screen.getByRole("tab", { name: "Ba" })).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{Home}");
    expect(screen.getByRole("tab", { name: "Một" })).toHaveFocus();
  });
});
