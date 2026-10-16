import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Button } from "./Button";

describe("Button", () => {
  it("still clicks through its hover and press springs", async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Lưu</Button>);
    const button = screen.getByRole("button", { name: "Lưu" });
    await userEvent.hover(button);
    await userEvent.click(button);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("is inert while pending: busy, disabled and no clicks", async () => {
    const onClick = vi.fn();
    render(
      <Button pending onClick={onClick}>
        Lưu
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Lưu" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
    await userEvent.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("keeps a link as the element when asChild, with the same variant data", () => {
    render(
      <Button asChild variant="outline">
        <a href="/x">Đi</a>
      </Button>,
    );
    const link = screen.getByRole("link", { name: "Đi" });
    expect(link).toHaveAttribute("data-variant", "outline");
  });
});
