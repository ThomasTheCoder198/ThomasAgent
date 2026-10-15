import { useTranslations } from "next-intl";

import type { ToolLine } from "@/components/lines";
import { LineDots } from "@/components/LineDots";
import { Badge } from "@/components/ui/Badge";

type Props = {
  kbName: string;
  /** One dot per tool source, coloured through `lines.ts`. */
  lines: readonly ToolLine[];
  toolCount: number;
  model?: string;
};

/** What defines an Agent at a glance: its Knowledge Base, its tools and the model; shared by the list rows and the settings header. */
export function AgentMeta({ kbName, lines, toolCount, model }: Props) {
  const t = useTranslations("agents");
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
      <Badge className="max-w-full min-w-0">
        <span aria-hidden className="bg-kb size-1.5 shrink-0 rounded-full" />
        <span className="truncate">{kbName}</span>
      </Badge>
      <span className="text-muted-foreground inline-flex items-center gap-2 text-sm">
        <LineDots lines={lines} size="md" />
        {t("toolCount", { count: toolCount })}
      </span>
      {model && <span className="tabular text-muted-foreground min-w-0 truncate text-xs">{model}</span>}
    </span>
  );
}
