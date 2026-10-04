/**
 * Line colours are the world's shared vocabulary: every tool source is a transit line, and the same
 * colour marks it in the sidebar, the run line, citation roundels and approval signs.
 * Class names are spelled out in full so Tailwind's scanner can see them.
 */
export const toolLines = ["kb", "mcp", "cmp", "app"] as const;
export type ToolLine = (typeof toolLines)[number];
export type Line = ToolLine | "model";

export function isToolLine(value: unknown): value is ToolLine {
  return toolLines.includes(value as ToolLine);
}

export function isLine(value: unknown): value is Line {
  return value === "model" || isToolLine(value);
}

export const lineBg: Record<Line, string> = {
  kb: "bg-kb",
  mcp: "bg-mcp",
  cmp: "bg-cmp",
  app: "bg-app",
  model: "bg-model",
};

export const lineBorder: Record<Line, string> = {
  kb: "border-kb",
  mcp: "border-mcp",
  cmp: "border-cmp",
  app: "border-app",
  model: "border-model",
};

export const lineText: Record<Line, string> = {
  kb: "text-kb",
  mcp: "text-mcp",
  cmp: "text-cmp",
  app: "text-app",
  model: "text-ink",
};

/** Text that sits on a solid line fill (roundels, line badges). */
export const onLine: Record<Line, string> = {
  kb: "text-on-kb",
  mcp: "text-on-mcp",
  cmp: "text-on-cmp",
  app: "text-on-app",
  model: "text-on-model",
};

export const lineTint: Record<Line, string> = {
  kb: "bg-kb-tint",
  mcp: "bg-mcp-tint",
  cmp: "bg-cmp-tint",
  app: "bg-app-tint",
  model: "bg-paper-2",
};
