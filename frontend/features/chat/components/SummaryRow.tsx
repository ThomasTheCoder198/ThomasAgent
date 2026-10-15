"use client";

import { ChevronDown } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import type { Line } from "@/components/lines";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";
import { formatSeconds } from "@/lib/format";

import { runLines, toolCount, totalDurationMs, type Station } from "../run-view";
import { MiniLine } from "./MiniLine";

type Props = {
  /** Only stations that actually ran; a tool waiting for approval is passed as `pendingLine`. */
  stations: Station[];
  pendingLine?: Line;
  expanded: boolean;
  onToggle: () => void;
  controls: string;
};

/** When a run's answer lands, its opening stations fold into this pill; the mini line keeps the route readable. */
export function SummaryRow({ stations, pendingLine, expanded, onToggle, controls }: Props) {
  const t = useTranslations("chat");
  const locale = useLocale();
  const seconds = formatSeconds(totalDurationMs(stations), locale);
  const tools = toolCount(stations);
  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={onToggle}
      aria-expanded={expanded}
      aria-controls={controls}
      aria-label={`${t("summary", { tools })} · ${t("seconds", { seconds })}`}
      title={expanded ? t("hideSteps") : t("showSteps")}
      className="text-muted-foreground hover:text-foreground mb-4 max-w-full rounded-full text-[0.8125rem] font-medium"
    >
      <MiniLine lines={runLines(stations)} pending={pendingLine} />
      <span className="hidden truncate sm:inline">{t("summary", { tools })}</span>
      <span className="truncate sm:hidden">{t("summaryShort", { tools })}</span>
      <span className="tabular text-xs">{t("seconds", { seconds })}</span>
      <ChevronDown aria-hidden className={cn("transition-transform", expanded && "rotate-180")} />
    </Button>
  );
}
