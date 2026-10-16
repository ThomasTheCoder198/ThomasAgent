import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { SegmentedChoice } from "./SegmentedChoice";

const options = [
  { value: "a", label: "Một" },
  { value: "b", label: "Hai" },
  { value: "c", label: "Ba" },
] as const;

function Harness() {
  const [value, setValue] = useState<"a" | "b" | "c">("a");
  return <SegmentedChoice legend="Chọn" value={value} options={options} onChange={setValue} />;
}

describe("SegmentedChoice", () => {
  it("selects with a click and never clears itself by pressing the chosen segment again", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("radio", { name: "Hai" }));
    expect(screen.getByRole("radio", { name: "Hai" })).toBeChecked();
    await userEvent.click(screen.getByRole("radio", { name: "Hai" }));
    expect(screen.getByRole("radio", { name: "Hai" })).toBeChecked();
  });

  it("moves with the arrow keys", async () => {
    render(<Harness />);
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("radio", { name: "Hai" })).toHaveFocus();
  });
});
