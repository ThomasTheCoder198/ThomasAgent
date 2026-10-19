import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { ProviderDialog } from "./ProviderDialog";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
const createProvider = vi.fn().mockResolvedValue({ id: "p9", name: "Mine" });
const syncProvider = vi.fn().mockResolvedValue({ added: 3, updated: 0, skipped: 0 });
vi.mock("../api", () => ({
  createProvider: (...a: unknown[]) => createProvider(...a),
  syncProvider: (...a: unknown[]) => syncProvider(...a),
  updateProvider: vi.fn(),
}));
vi.mock("@/lib/useNotify", () => ({ useNotify: () => ({ success: vi.fn(), error: vi.fn() }) }));

const wrap = (node: React.ReactNode) =>
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      {node}
    </NextIntlClientProvider>,
  );

describe("ProviderDialog", () => {
  it("hides the base URL for built-in kinds and creates then syncs", async () => {
    wrap(<ProviderDialog onOpenChange={vi.fn()} />);
    expect(screen.queryByRole("textbox", { name: messages.settings.baseUrl })).toBeNull();
    await userEvent.type(screen.getByRole("textbox", { name: messages.settings.providerName }), "Mine");
    await userEvent.type(screen.getByLabelText(messages.settings.apiKey), "sk-or-1");
    await userEvent.click(screen.getByRole("button", { name: messages.settings.saveAndSync }));
    expect(createProvider).toHaveBeenCalledWith(
      { kind: "openrouter", name: "Mine", apiKey: "sk-or-1" },
      "en",
    );
    expect(syncProvider).toHaveBeenCalledWith("p9", "en");
  });
});
