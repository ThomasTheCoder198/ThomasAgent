import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import messages from "@/messages/vi.json";

import type { AgentDetail } from "../contract";
import { fromDetail } from "../draft";
import { KnowledgeTab } from "./KnowledgeTab";

const agent: AgentDetail = {
  id: "a1",
  name: "A",
  description: "",
  instructions: "",
  kbId: "kb-gone",
  toolSources: [],
  modelOverrides: {},
  contextFiles: [],
  updatedAt: "",
};

function renderTab(knowledgeBases: Parameters<typeof KnowledgeTab>[0]["knowledgeBases"]) {
  render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <KnowledgeTab
        agent={agent}
        draft={fromDetail(agent)}
        update={() => undefined}
        errors={{}}
        knowledgeBases={knowledgeBases}
      />
    </NextIntlClientProvider>,
  );
}

describe("KnowledgeTab", () => {
  it("shows a checked 'unknown' row when the bound KB is not in the list", () => {
    renderTab({ ok: true, data: [{ id: "kb-1", name: "Chính sách", documents: 3, updatedAt: "" }] });
    const unknown = screen.getByRole("radio", { name: /Knowledge Base không xác định/ });
    expect(unknown).toBeChecked();
    expect(screen.getByText(/kb-gone/)).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Chính sách/ })).not.toBeChecked();
  });

  it("shows the load failure instead of an empty list", () => {
    renderTab({ ok: false, message: "Lỗi máy chủ" });
    expect(screen.getByRole("alert")).toHaveTextContent("Không tải được danh sách Knowledge Base.");
    expect(screen.getByRole("alert")).toHaveTextContent("Lỗi máy chủ");
    expect(screen.queryByRole("radio")).toBeNull();
  });
});
