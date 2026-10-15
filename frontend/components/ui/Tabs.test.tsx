import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "./Tabs";

function Harness() {
  const [value, setValue] = useState("a");
  return (
    <Tabs value={value} onValueChange={setValue}>
      <TabsList aria-label="Thử">
        <TabsTrigger value="a">Một</TabsTrigger>
        <TabsTrigger value="b">Hai</TabsTrigger>
        <TabsTrigger value="c">Ba</TabsTrigger>
      </TabsList>
      <TabsContent value="a">Nội dung một</TabsContent>
      <TabsContent value="b">Nội dung hai</TabsContent>
      <TabsContent value="c">Nội dung ba</TabsContent>
    </Tabs>
  );
}

describe("Tabs keyboard", () => {
  it("marks the selected tab", () => {
    render(<Harness />);
    expect(screen.getByRole("tab", { name: "Một" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Hai" })).toHaveAttribute("aria-selected", "false");
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

  it("shows only the selected panel and names it by its tab", async () => {
    render(<Harness />);
    expect(screen.getByRole("tabpanel", { name: "Một" })).toHaveTextContent("Nội dung một");
    await userEvent.click(screen.getByRole("tab", { name: "Hai" }));
    expect(screen.getByRole("tabpanel", { name: "Hai" })).toHaveTextContent("Nội dung hai");
    expect(screen.queryByText("Nội dung một")).toBeNull();
  });
});
