import { act, fireEvent, render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import messages from "@/messages/en.json";

import { ConfirmDelete } from "./ConfirmDelete";

const notify = { success: vi.fn(), error: vi.fn(), info: vi.fn(), problem: vi.fn() };
vi.mock("@/lib/useNotify", () => ({ useNotify: () => notify }));

const onOpenChange = vi.fn();
const onConfirm = vi.fn();

function renderConfirm() {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <ConfirmDelete
        onOpenChange={onOpenChange}
        title="Delete a.md?"
        body="Gone."
        successMessage="Deleted a.md."
        onConfirm={onConfirm}
      />
    </NextIntlClientProvider>,
  );
}

const clickDelete = () =>
  act(async () => void fireEvent.click(screen.getByRole("button", { name: messages.kb.delete })));

beforeEach(() => vi.clearAllMocks());

describe("ConfirmDelete", () => {
  it("toasts the success and closes when the item is gone", async () => {
    onConfirm.mockResolvedValue(undefined);
    renderConfirm();
    await clickDelete();
    expect(notify.success).toHaveBeenCalledWith("Deleted a.md.");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("toasts the failure from its error and stays open for another try", async () => {
    const error = new ApiError(ErrorCode.INTERNAL_ERROR, "Down.", 503, undefined, "req_abcdefgh");
    onConfirm.mockRejectedValue(error);
    renderConfirm();
    await clickDelete();
    expect(notify.error).toHaveBeenCalledWith(error);
    expect(notify.success).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: messages.kb.delete })).toBeEnabled();
  });
});
