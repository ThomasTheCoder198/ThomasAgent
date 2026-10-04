import { describe, expect, it } from "vitest";

import { remarkCitations } from "./remark-citations";

function paragraph(value: string) {
  return { type: "root", children: [{ type: "paragraph", children: [{ type: "text", value }] }] };
}

describe("remarkCitations", () => {
  it("splits known markers into cite-ref nodes", () => {
    const tree = paragraph("Thời hạn 14 ngày [1], phí 5% [4].");
    remarkCitations({ known: new Set([1, 4]) })(tree);
    const children = tree.children[0]?.children ?? [];
    expect(children.map((c) => c.type)).toEqual(["text", "citeRef", "text", "citeRef", "text"]);
    expect(children[1]).toMatchObject({ data: { hName: "cite-ref", hProperties: { n: 1 } } });
  });

  it("leaves unknown markers as plain text so the model cannot invent a source", () => {
    const tree = paragraph("Không có nguồn [9].");
    remarkCitations({ known: new Set([1]) })(tree);
    expect(tree.children[0]?.children).toEqual([{ type: "text", value: "Không có nguồn [9]." }]);
  });
});
