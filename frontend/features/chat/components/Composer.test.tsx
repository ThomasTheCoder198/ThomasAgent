import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { Composer } from "./Composer";

function renderComposer(overrides: Partial<React.ComponentProps<typeof Composer>> = {}) {
  const props = {
    busy: false,
    onSend: vi.fn(),
    onStop: vi.fn(),
    models: [],
    modelId: undefined,
    onModelChange: vi.fn(),
    scope: null,
    contextRatio: 0,
    ...overrides,
  };
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <Composer {...props} />
    </NextIntlClientProvider>,
  );
  return props;
}

describe("Composer", () => {
  it("sends the trimmed text on Enter and clears the field", async () => {
    const { onSend } = renderComposer();
    const field = screen.getByRole("textbox", { name: "Message" });
    await userEvent.type(field, "  hello  {Enter}");
    expect(onSend).toHaveBeenCalledExactlyOnceWith("hello");
    expect(field).toHaveValue("");
  });

  it("keeps Shift+Enter for a new line", async () => {
    const { onSend } = renderComposer();
    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "a{Shift>}{Enter}{/Shift}b");
    expect(onSend).not.toHaveBeenCalled();
  });

  it("never sends half-composed IME text", () => {
    const { onSend } = renderComposer();
    const field = screen.getByRole("textbox", { name: "Message" });
    fireEvent.change(field, { target: { value: "xin chao" } });
    fireEvent.keyDown(field, { key: "Enter", isComposing: true });
    expect(onSend).not.toHaveBeenCalled();
  });

  it("disables send until there is text", async () => {
    renderComposer();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "hi");
    expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  });

  it("swaps send for stop while the agent runs", async () => {
    const { onStop } = renderComposer({ busy: true });
    expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    expect(onStop).toHaveBeenCalledOnce();
  });
});
