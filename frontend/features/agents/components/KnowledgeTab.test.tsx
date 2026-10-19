import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import type { Agent } from "@/lib/core/types";
import type { KnowledgeBase } from "@/lib/ragx/types";
import messages from "@/messages/vi.json";

import { fromAgent } from "../draft";
import { KnowledgeTab } from "./KnowledgeTab";

const agent: Agent = {
  id: "a1",
  name: "A",
  description: "",
  instructions: "",
  kbId: "kb-gone",
  kbName: "",
  kbMissing: false,
  modelOverrides: {},
  createdAt: "",
  updatedAt: "",
};

const kb = (id: string, name: string): KnowledgeBase => ({
  id,
  name,
  isDefault: false,
  embeddingModel: "m",
  embeddingDims: 3,
  fileCount: 3,
  createdAt: "",
  updatedAt: "",
});

function renderTab(
  knowledgeBases: Parameters<typeof KnowledgeTab>[0]["knowledgeBases"],
  stored: Agent = agent,
) {
  render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <KnowledgeTab
        orgId="o1"
        agent={stored}
        draft={fromAgent(stored)}
        update={() => undefined}
        errors={{}}
        knowledgeBases={knowledgeBases}
      />
    </NextIntlClientProvider>,
  );
}

describe("KnowledgeTab", () => {
  it("keeps the binding visible as 'unknown' when the bound KB is not in the list", () => {
    renderTab({ ok: true, data: [kb("kb-1", "Chính sách")] });
    expect(screen.getByRole("combobox", { name: "Knowledge" })).toHaveTextContent(
      "Knowledge Base không xác định",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(/kb-gone/);
  });

  it("says the Knowledge Base was deleted when core flags it missing", () => {
    renderTab({ ok: true, data: [kb("kb-1", "Chính sách")] }, { ...agent, kbMissing: true });
    expect(screen.getByRole("alert")).toHaveTextContent("Knowledge Base của agent này đã bị xoá.");
  });

  it("shows the bound KB by name, without a warning, when the list contains it", () => {
    renderTab({ ok: true, data: [kb("kb-gone", "Chính sách")] });
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
