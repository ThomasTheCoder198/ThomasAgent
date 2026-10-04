import { AppWindow, LibraryBig, Plug, Workflow, type LucideIcon } from "lucide-react";

import type { ToolLine } from "./lines";

/**
 * One glyph per tool line, shared by the run line, approval signs and Agent settings so a source looks the
 * same everywhere: KB = library, MCP = plug, Composio = workflow, Apps = app window.
 */
export const lineIcons: Record<ToolLine, LucideIcon> = {
  kb: LibraryBig,
  mcp: Plug,
  cmp: Workflow,
  app: AppWindow,
};
