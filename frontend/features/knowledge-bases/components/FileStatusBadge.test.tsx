import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { fileStatuses } from "../contract";
import { FileStatusBadge } from "./FileStatusBadge";

// jsdom has no canvas.
vi.mock("thinking-orbs", () => ({ ThinkingOrb: () => <span data-testid="orb" /> }));

function renderBadge(status: (typeof fileStatuses)[number], errorCode = "") {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <FileStatusBadge status={status} errorCode={errorCode} />
    </NextIntlClientProvider>,
  );
}

const WORKING = ["parsing", "enriching", "chunking", "embedding", "indexing"] as const;

describe("FileStatusBadge", () => {
  it("shows ready as the success badge, with no orb and no note", () => {
    renderBadge("ready");
    expect(screen.getByText(messages.kb.status.ready)).toHaveAttribute("data-variant", "success");
    expect(screen.queryByTestId("orb")).not.toBeInTheDocument();
    expect(screen.queryByText(messages.kb.degradedNote)).not.toBeInTheDocument();
  });

  it("shows a degraded file as ready, with its local-parse note beside the badge", () => {
    renderBadge("degraded");
    expect(screen.getByText(messages.kb.status.ready)).toHaveAttribute("data-variant", "success");
    expect(screen.getByText(messages.kb.degradedNote)).toBeInTheDocument();
  });

  it("shows failed as the destructive badge with the error code in mono", () => {
    renderBadge("failed", "INGEST_PARSE_FAILED");
    expect(screen.getByText(messages.kb.status.failed)).toHaveAttribute("data-variant", "destructive");
    expect(screen.getByText("INGEST_PARSE_FAILED").className).toMatch(/tabular/);
  });

  it.each(WORKING)("shows %s as a quiet badge with its step and the working orb", (status) => {
    renderBadge(status);
    expect(screen.getByText(messages.kb.status[status]).closest("[data-slot=badge]")).toHaveAttribute(
      "data-variant",
      "default",
    );
    expect(screen.getByTestId("orb")).toBeInTheDocument();
  });

  it("shows a waiting file as a quiet badge without an orb, because nothing is working on it yet", () => {
    renderBadge("pending");
    expect(screen.getByText(messages.kb.status.pending)).toBeInTheDocument();
    expect(screen.queryByTestId("orb")).not.toBeInTheDocument();
  });

  it("has a label for every status that is not a degraded ready", () => {
    expect(fileStatuses.filter((s) => s !== "degraded")).toHaveLength(Object.keys(messages.kb.status).length);
  });
});
