import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import messages from "@/messages/en.json";

import { NewAgentDialog } from "./NewAgentDialog";

const notify = { success: vi.fn(), error: vi.fn(), info: vi.fn(), problem: vi.fn() };
vi.mock("@/lib/useNotify", () => ({ useNotify: () => notify }));
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, refresh: vi.fn() }) }));
const createAgent = vi.fn();
vi.mock("../api", () => ({ createAgent: (...args: unknown[]) => createAgent(...args) }));

const kbs = [
  { id: "kb-1", name: "Chính sách" },
  { id: "kb-2", name: "FAQ" },
];

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <NewAgentDialog orgId="o1" knowledgeBases={kbs as never} onOpenChange={onOpenChange} />
    </NextIntlClientProvider>,
  );
  return { onOpenChange };
}

const submit = () =>
  act(async () => void fireEvent.click(screen.getByRole("button", { name: messages.agents.create })));

beforeEach(() => vi.clearAllMocks());

describe("NewAgentDialog", () => {
  it("needs a name before it calls core", async () => {
    renderDialog();
    await submit();
    expect(createAgent).not.toHaveBeenCalled();
    expect(screen.getByText(messages.agents.nameRequired)).toBeVisible();
  });

  it("creates the agent on the first KB by default and opens it", async () => {
    createAgent.mockResolvedValue({ id: "a9", name: "Pháp chế" });
    renderDialog();
    await userEvent.type(screen.getByRole("textbox", { name: messages.agents.name }), "  Pháp chế ");
    await submit();
    expect(createAgent).toHaveBeenCalledWith("o1", { name: "Pháp chế", description: "", kbId: "kb-1" }, "en");
    expect(push).toHaveBeenCalledWith("/agents/a9");
    expect(notify.success).toHaveBeenCalled();
  });

  it("keeps a taken name on the field instead of a toast", async () => {
    createAgent.mockRejectedValue(
      new ApiError(ErrorCode.CONFLICT, "taken", 409, { fields: [{ field: "name", code: "CONFLICT" }] }),
    );
    renderDialog();
    await userEvent.type(screen.getByRole("textbox", { name: messages.agents.name }), "Pháp chế");
    await submit();
    expect(screen.getByText(messages.agents.nameTaken)).toBeVisible();
    expect(notify.error).not.toHaveBeenCalled();
  });

  it("clears a taken-name error when the name changes", async () => {
    createAgent.mockRejectedValue(
      new ApiError(ErrorCode.CONFLICT, "taken", 409, { fields: [{ field: "name", code: "CONFLICT" }] }),
    );
    renderDialog();
    const name = screen.getByRole("textbox", { name: messages.agents.name });
    await userEvent.type(name, "Pháp chế");
    await submit();
    expect(screen.getByText(messages.agents.nameTaken)).toBeVisible();

    await userEvent.type(name, " mới");
    expect(screen.queryByText(messages.agents.nameTaken)).not.toBeInTheDocument();
  });

  it("explains that a KB is needed when the org has none", () => {
    render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <NewAgentDialog orgId="o1" knowledgeBases={[]} onOpenChange={vi.fn()} />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText(messages.agents.needKb)).toBeVisible();
    expect(screen.getByRole("button", { name: messages.agents.create })).toBeDisabled();
  });
});
