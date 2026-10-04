/**
 * Turns the model's `[n]` citation markers into `<cite-ref n="…">` elements that the answer renders as
 * roundels. Markers only become roundels when a source with that number exists in the message.
 * Each roundel is wrapped with the word before it and any punctuation after it in a no-wrap span, so a
 * roundel never starts a line on its own (browsers may break before an inline-block even after U+00A0).
 */
type Node = { type: string; value?: string; children?: Node[]; data?: Record<string, unknown> };

export const CITE_TAG = "cite-ref";
export const CITE_GROUP_CLASS = "whitespace-nowrap";
const MARKER = /\[(\d+)\]([.,;:!?)]*)/g;
const LAST_WORD = /(\S+)(\s*)$/;
const NO_BREAK_SPACE = " ";

const text = (value: string): Node => ({ type: "text", value });

function citeRef(n: number): Node {
  return { type: "citeRef", children: [], data: { hName: CITE_TAG, hProperties: { n } } };
}

function group(children: Node[]): Node {
  return {
    type: "citeGroup",
    children,
    data: { hName: "span", hProperties: { className: [CITE_GROUP_CLASS] } },
  };
}

/** Splits the text before a marker into what stays free and the last word that travels with the roundel. */
function detachLastWord(before: string): { free: string; word: string } {
  const match = LAST_WORD.exec(before);
  if (!match) return { free: before, word: "" };
  const [whole, word = "", space = ""] = match;
  return {
    free: before.slice(0, before.length - whole.length),
    word: space ? `${word}${NO_BREAK_SPACE}` : word,
  };
}

function split(value: string, known: ReadonlySet<number>): Node[] {
  const out: Node[] = [];
  let last = 0;
  for (const match of value.matchAll(MARKER)) {
    const n = Number(match[1]);
    if (!known.has(n)) continue;
    const { free, word } = detachLastWord(value.slice(last, match.index));
    if (free) out.push(text(free));
    const punctuation = match[2] ?? "";
    out.push(group([...(word ? [text(word)] : []), citeRef(n), ...(punctuation ? [text(punctuation)] : [])]));
    last = match.index + match[0].length;
  }
  if (last === 0) return [text(value)];
  if (last < value.length) out.push(text(value.slice(last)));
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
