import { describe, expect, it } from "vitest";

import { remarkCitations } from "./remark-citations";

type TestNode = { type: string; value?: string; children?: TestNode[]; data?: Record<string, unknown> };

function paragraph(value: string): { type: string; children: TestNode[] } {
  return { type: "root", children: [{ type: "paragraph", children: [{ type: "text", value }] }] };
}

function inline(tree: { children: TestNode[] }): TestNode[] {
  return tree.children[0]?.children ?? [];
}

describe("remarkCitations", () => {
  it("turns known markers into cite-ref nodes inside no-wrap groups", () => {
    const tree = paragraph("Thời hạn 14 ngày [1], phí 5% [4].");
    remarkCitations({ known: new Set([1, 4]) })(tree);
    const children = inline(tree);
    expect(children.map((c) => c.type)).toEqual(["text", "citeGroup", "text", "citeGroup"]);
    expect(children[1]?.children?.[1]).toMatchObject({ data: { hName: "cite-ref", hProperties: { n: 1 } } });
  });

  it("keeps the last word, the roundel and its punctuation together", () => {
    const tree = paragraph("không phụ thuộc thời hạn [5].");
    remarkCitations({ known: new Set([5]) })(tree);
    const [free, grouped] = inline(tree);
    expect(free).toEqual({ type: "text", value: "không phụ thuộc thời " });
    expect(grouped?.data).toMatchObject({ hName: "span", hProperties: { className: ["whitespace-nowrap"] } });
    expect(grouped?.children?.map((c) => c.value ?? c.type)).toEqual(["hạn ", "citeRef", "."]);
  });

  it("leaves unknown markers as plain text so the model cannot invent a source", () => {
    const tree = paragraph("Không có nguồn [9].");
    remarkCitations({ known: new Set([1]) })(tree);
    expect(inline(tree)).toEqual([{ type: "text", value: "Không có nguồn [9]." }]);
  });
});
