import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import messages from "@/messages/vi.json";

import { SaveBar } from "./SaveBar";

function renderBar(props: Partial<Parameters<typeof SaveBar>[0]>) {
  return render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <SaveBar
        dirty={false}
        canSave={false}
        errors={{}}
        save={{ status: "idle" }}
        onSave={() => undefined}
        onDiscard={() => undefined}
        {...props}
      />
    </NextIntlClientProvider>,
  );
}

describe("SaveBar", () => {
  it("leaves the accessibility tree and says nothing while hidden", () => {
    const { container } = renderBar({});
    const bar = container.firstElementChild;
    expect(bar).toHaveAttribute("inert");
    expect(bar).toHaveAttribute("aria-hidden", "true");
    expect(screen.queryByText("Có thay đổi chưa lưu")).toBeNull();
  });

  it("announces unsaved changes and offers save when dirty", () => {
    renderBar({ dirty: true, canSave: true });
    expect(screen.getByText("Có thay đổi chưa lưu")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeEnabled();
  });

  it("blocks saving and explains why when the name is empty", () => {
    renderBar({ dirty: true, canSave: false, errors: { name: "required" } });
    expect(screen.getByText("Tên agent không được để trống.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeDisabled();
  });
});
