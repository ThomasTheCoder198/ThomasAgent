import { render, screen } from "@testing-library/react";
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
  it("shows a storage failure with the S3 code in mono and no request id", () => {
    renderRow(failed({ failedStep: "storage", error: new StorageUploadError(400, "EntityTooLarge") }));
    const code = screen.getByText("EntityTooLarge");
    expect(code.className).toMatch(/tabular/);
    expect(screen.getByRole("alert")).toHaveTextContent("Storage refused the upload (EntityTooLarge).");
    expect(screen.queryByText(/Request ID/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.retry })).toBeInTheDocument();
  });

  it("says storage could not be reached, not refused, for a network failure", () => {
    renderRow(failed({ failedStep: "storage", error: new StorageUploadError(0, "NETWORK") }));
    expect(screen.getByRole("alert")).toHaveTextContent("Could not reach file storage (NETWORK).");
    expect(screen.getByText("NETWORK").className).toMatch(/tabular/);
    expect(screen.getByRole("alert")).not.toHaveTextContent(/refused/);
  });

  it("hides Cancel while the upload is finishing, because aborting after ragx committed hides the file", () => {
    renderRow({ ...failed({}), state: "confirming" });
    expect(screen.getByText(messages.kb.confirming)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: messages.kb.cancel })).not.toBeInTheDocument();
    renderRow({ ...failed({}), id: "i2", state: "uploading" });
    expect(screen.getByRole("button", { name: messages.kb.cancel })).toBeInTheDocument();
  });

  it("shows a presign or confirm failure with its request id", () => {
    const error = new ApiError(ErrorCode.INTERNAL_ERROR, "Service down.", 503, undefined, "req_abcdefgh");
    renderRow(failed({ failedStep: "confirm", error }));
    expect(screen.getByRole("alert")).toHaveTextContent("Service down. (Request ID: req_abcdefgh)");
  });

  it("offers Dismiss only for a file that failed the client check", () => {
    renderRow(failed({ problem: "type" }));
    expect(screen.getByText(messages.kb.problem.type)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: messages.kb.retry })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: messages.kb.dismiss })).toBeInTheDocument();
  });
});
