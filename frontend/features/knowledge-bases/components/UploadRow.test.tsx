import { fireEvent, render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import messages from "@/messages/en.json";

import { StorageUploadError } from "../upload";
import type { QueueItem } from "../useUploadQueue";
import { UploadRow } from "./UploadRow";

const handlers = { onCancel: vi.fn(), onRetry: vi.fn(), onDismiss: vi.fn() };

const failed = (over: Partial<QueueItem>): QueueItem => ({
  id: "i1",
  file: new File(["abc"], "a.md"),
  state: "failed",
  loaded: 0,
  ...over,
});

function renderRow(item: QueueItem) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <UploadRow item={item} {...handlers} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("UploadRow failures", () => {
  // P-F17: the toast announces a storage failure (role=alert); the row keeps it in place, quietly, beside Retry.
  it("keeps a storage failure in the row with the S3 code in mono, no request id and no second alert", () => {
    renderRow(failed({ failedStep: "storage", error: new StorageUploadError(400, "EntityTooLarge") }));
    const code = screen.getByText("EntityTooLarge");
    expect(code.className).toMatch(/tabular/);
    expect(screen.getByText(/The file changed after it was picked/)).toHaveTextContent(
      "The file changed after it was picked. Pick it again (EntityTooLarge).",
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText(/Request ID/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.retry })).toBeInTheDocument();
  });

  it("says storage could not be reached, not refused, for a network failure", () => {
    renderRow(failed({ failedStep: "storage", error: new StorageUploadError(0, "NETWORK") }));
    const line = screen.getByText(/Could not reach file storage/);
    expect(line).toHaveTextContent(
      "Could not reach file storage. Check the connection, then retry (NETWORK).",
    );
    expect(screen.getByText("NETWORK").className).toMatch(/tabular/);
    expect(line).not.toHaveTextContent(/refused/);
  });

  it("hides Cancel while the upload is finishing, because aborting after ragx committed hides the file", () => {
    renderRow({ ...failed({}), state: "confirming" });
    expect(screen.getByText(messages.kb.confirming)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: messages.kb.cancel })).not.toBeInTheDocument();
    renderRow({ ...failed({}), id: "i2", state: "uploading" });
    expect(screen.getByRole("button", { name: messages.kb.cancel })).toBeInTheDocument();
  });

  it("states a presign or confirm failure quietly with its request id, because its toast already alerted", () => {
    const error = new ApiError(ErrorCode.INTERNAL_ERROR, "Service down.", 503, undefined, "req_abcdefgh");
    renderRow(failed({ failedStep: "confirm", error }));
    expect(screen.getByText("Service down. (Request ID: req_abcdefgh)")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.retry })).toBeInTheDocument();
  });

  it("offers Retry and Dismiss as labelled buttons on a failed row", () => {
    renderRow(failed({ failedStep: "storage", error: new StorageUploadError(503, "SlowDown") }));
    fireEvent.click(screen.getByRole("button", { name: messages.kb.retry }));
    fireEvent.click(screen.getByRole("button", { name: messages.kb.dismiss }));
    expect(handlers.onRetry).toHaveBeenCalledTimes(1);
    expect(handlers.onDismiss).toHaveBeenCalledTimes(1);
  });

  it("offers Dismiss only for a file that failed the client check, and alerts because no toast did", () => {
    renderRow(failed({ problem: "type" }));
    expect(screen.getByRole("alert")).toHaveTextContent(messages.kb.problem.type);
    expect(screen.queryByRole("button", { name: messages.kb.retry })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.dismiss })).toBeInTheDocument();
  });

  it("shows byte progress as a progressbar with a mono percentage", () => {
    renderRow({ ...failed({}), state: "uploading", file: new File(["abcd"], "a.md"), loaded: 2 });
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "50");
    expect(screen.getByText("50%").className).toMatch(/tabular/);
  });
});
