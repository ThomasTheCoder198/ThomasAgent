import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import messages from "@/messages/en.json";

import { fileStatuses } from "../contract";
import { FileStatusMark } from "./FileStatusMark";

function renderMark(status: (typeof fileStatuses)[number], errorCode = "") {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <FileStatusMark status={status} errorCode={errorCode} />
    </NextIntlClientProvider>,
  );
}

const READY_RING = ["ready", "degraded"];
const inProgress = fileStatuses.filter((s) => !READY_RING.includes(s) && s !== "failed");

describe("FileStatusMark", () => {
  it.each(READY_RING)("draws %s with the KB ring (a degraded file is ready, parsed locally)", (status) => {
    renderMark(status as "ready" | "degraded");
    expect(screen.getByRole("img", { name: messages.kb.status[status as "ready"] }).className).toMatch(
      /border-kb/,
    );
  });

  it("draws failed with the Apps ring and the error code", () => {
    renderMark("failed", "INGEST_PARSE_FAILED");
    expect(screen.getByRole("img", { name: messages.kb.status.failed }).className).toMatch(/border-app/);
    expect(screen.getByText("INGEST_PARSE_FAILED")).toBeInTheDocument();
  });

  it.each(inProgress)("draws %s as a hollow ring with its step label", (status) => {
    renderMark(status);
    const mark = screen.getByRole("img", { name: messages.kb.status[status] });
    expect(mark.className).toMatch(/border-border/);
    expect(screen.getByText(messages.kb.status[status])).toBeInTheDocument();
  });

  it("has a mark for every status", () => {
    expect(fileStatuses).toHaveLength(Object.keys(messages.kb.status).length);
  });
});
