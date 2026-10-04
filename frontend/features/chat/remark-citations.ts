/**
 * Turns the model's `[n]` citation markers into `<cite-ref n="…">` elements that the answer renders as
 * roundels. Markers only become roundels when a source with that number exists in the message.
 */
type Node = { type: string; value?: string; children?: Node[]; data?: Record<string, unknown> };

export const CITE_TAG = "cite-ref";
const MARKER = /\[(\d+)\]/g;

function split(text: string, known: ReadonlySet<number>): Node[] {
  const out: Node[] = [];
  let last = 0;
  for (const match of text.matchAll(MARKER)) {
    const n = Number(match[1]);
    if (!known.has(n)) continue;
    const start = match.index;
    if (start > last) out.push({ type: "text", value: text.slice(last, start) });
    out.push({ type: "citeRef", children: [], data: { hName: CITE_TAG, hProperties: { n } } });
    last = start + match[0].length;
  }
  if (last === 0) return [{ type: "text", value: text }];
  if (last < text.length) out.push({ type: "text", value: text.slice(last) });
  return out;
}

function walk(node: Node, known: ReadonlySet<number>) {
  if (!node.children) return;
  node.children = node.children.flatMap((child) => {
    if (child.type === "text" && child.value) return split(child.value, known);
    walk(child, known);
    return [child];
  });
}

export function remarkCitations(options: { known: ReadonlySet<number> }) {
  return (tree: Node) => walk(tree, options.known);
}
