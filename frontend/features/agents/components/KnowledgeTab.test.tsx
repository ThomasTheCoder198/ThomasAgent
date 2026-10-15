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
  it("keeps the binding visible as 'unknown' when the bound KB is not in the list", () => {
    renderTab({ ok: true, data: [{ id: "kb-1", name: "Chính sách", documents: 3, updatedAt: "" }] });
    expect(screen.getByRole("combobox", { name: "Knowledge" })).toHaveTextContent(
      "Knowledge Base không xác định",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(/kb-gone/);
  });

  it("shows the bound KB by name, without a warning, when the list contains it", () => {
    renderTab({ ok: true, data: [{ id: "kb-gone", name: "Chính sách", documents: 3, updatedAt: "" }] });
    expect(screen.getByRole("combobox", { name: "Knowledge" })).toHaveTextContent(/Chính sách/);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows the load failure instead of an empty list", () => {
    renderTab({ ok: false, message: "Lỗi máy chủ" });
    expect(screen.getByRole("alert")).toHaveTextContent("Không tải được danh sách Knowledge Base.");
    expect(screen.getByRole("alert")).toHaveTextContent("Lỗi máy chủ");
    expect(screen.queryByRole("combobox")).toBeNull();
  });
});
