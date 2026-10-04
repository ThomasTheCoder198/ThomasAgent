import { render, screen } from "@testing-library/react";
import { MessageSquarePlus } from "lucide-react";
import { describe, expect, it } from "vitest";

import { Sidebar } from "./Sidebar";

const labels = {
  newChat: "Hội thoại mới",
  search: "Tìm kiếm",
  searchSoon: "Bảng lệnh ⌘K có ở M2",
  recents: "Gần đây",
  recentsEmpty: "Hội thoại gần đây sẽ hiện ở đây.",
  tenant: "Self-hosted · 1 tenant",
  nav: "Điều hướng chính",
};
const user = { id: "u1", email: "thomas@example.com", displayName: "Thomas" };

describe("Sidebar", () => {
  it("marks the active item", () => {
    render(
      <Sidebar
        items={[
          { href: "/chat", label: "Chat", icon: MessageSquarePlus },
          { href: "/tools", label: "Tools", icon: MessageSquarePlus, lines: ["mcp"] },
        ]}
        recents={[]}
        user={user}
        labels={labels}
        activeHref="/tools"
      />,
    );
    expect(screen.getByRole("link", { name: /Tools/ })).toHaveAttribute("aria-current", "page");
    expect(screen.getByText("Hội thoại gần đây sẽ hiện ở đây.")).toBeInTheDocument();
  });

  it("truncates long recent titles", () => {
    const title = "Phân tích chi tiết điều khoản hoàn tiền quốc tế cho đơn hàng vận chuyển đường biển 2026";
    render(
      <Sidebar
        items={[]}
        recents={[{ id: "c1", title, lines: ["kb"] }]}
        user={user}
        labels={labels}
        activeHref="/chat"
      />,
    );
    const el = screen.getByText(title);
    expect(el.className).toMatch(/truncate/);
    expect(el).toHaveAttribute("title", title);
  });

  it("marks the open conversation in recents", () => {
    render(
      <Sidebar
        items={[]}
        recents={[{ id: "c1", title: "Chính sách hoàn tiền 2026", lines: ["kb", "mcp"] }]}
        user={user}
        labels={labels}
        activeHref="/chat/c1"
      />,
    );
    expect(screen.getByRole("link", { name: /Chính sách hoàn tiền 2026/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});
