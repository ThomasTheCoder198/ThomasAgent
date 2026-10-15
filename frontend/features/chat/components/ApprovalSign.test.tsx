import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import type { ToolStation } from "../run-view";
import { ApprovalSign } from "./ApprovalSign";

const station: ToolStation = {
  kind: "tool",
  id: "t1",
  name: "composio_send_mail",
  title: "Send mail",
  line: "cmp",
  source: "Gmail",
  state: "awaiting",
  input: { to: "a@b.vn", cc: ["c@d.vn", "e@f.vn"] },
  approvalId: "ap-1",
  ownerApproved: false,
};

function renderSign(onDecide = vi.fn(), disabled = false) {
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <ApprovalSign station={station} onDecide={onDecide} disabled={disabled} />
    </NextIntlClientProvider>,
  );
  return onDecide;
}

describe("ApprovalSign", () => {
  it("wears the tool's own line colour, not a fixed one", () => {
    renderSign();
    const card = screen.getByRole("region", { name: "Needs your approval" });
    expect(card.querySelector(".bg-cmp")).not.toBeNull();
    expect(card.querySelector(".bg-mcp, .bg-mcp-tint")).toBeNull();
    expect(card).not.toHaveClass("bg-mcp-tint");
  });

  it("lists the real arguments", () => {
    renderSign();
    expect(screen.getByText("a@b.vn")).toBeInTheDocument();
    expect(screen.getByText("c@d.vn, e@f.vn")).toBeInTheDocument();
  });

  it("answers allow, deny and always allow with the approval id", async () => {
    const onDecide = renderSign();
    await userEvent.click(screen.getByRole("button", { name: "Allow" }));
    await userEvent.click(screen.getByRole("button", { name: "Deny" }));
    await userEvent.click(screen.getByRole("button", { name: "Always allow" }));
    expect(onDecide.mock.calls).toEqual([
      [{ approvalId: "ap-1", approved: true, always: false }],
      [{ approvalId: "ap-1", approved: false, always: false }],
      [{ approvalId: "ap-1", approved: true, always: true }],
    ]);
  });

  it("locks the answers while the run is still streaming", () => {
    renderSign(vi.fn(), true);
    expect(screen.getByRole("button", { name: "Allow" })).toBeDisabled();
  });
});
