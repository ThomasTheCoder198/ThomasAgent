import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { CapabilityDialog } from "./CapabilityDialog";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
const updateCapabilities = vi.fn().mockResolvedValue({});
vi.mock("../api", () => ({ updateCapabilities: (...a: unknown[]) => updateCapabilities(...a) }));
vi.mock("@/lib/useNotify", () => ({ useNotify: () => ({ success: vi.fn(), error: vi.fn() }) }));

const model = {
  id: "m2",
  providerId: "p1",
  modelRef: "a/two",
  displayName: "Two",
  capabilities: [],
  contextWindow: null,
  source: "sync",
} as const;

describe("CapabilityDialog", () => {
  it("saves the capabilities the operator switched on", async () => {
    const onOpenChange = vi.fn();
    render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <CapabilityDialog model={model as never} onOpenChange={onOpenChange} />
      </NextIntlClientProvider>,
    );
    expect(screen.getByRole("button", { name: messages.common.save })).toBeDisabled();
    await userEvent.click(screen.getByRole("switch", { name: "chat" }));
    await userEvent.click(screen.getByRole("switch", { name: "tools" }));
    await userEvent.click(screen.getByRole("button", { name: messages.common.save }));
    expect(updateCapabilities).toHaveBeenCalledWith("m2", ["chat", "tools"], "en");
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(refresh).toHaveBeenCalled();
  });
});
