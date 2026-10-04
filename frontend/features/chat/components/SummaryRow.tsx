"use client";

import { ChevronDown } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import type { Line } from "@/components/metro/lines";
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
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={expanded}
      aria-controls={controls}
      aria-label={`${t("summary", { tools })} · ${t("seconds", { seconds })}`}
      title={expanded ? t("hideSteps") : t("showSteps")}
      className="border-rule bg-paper text-ink-2 hover:border-rule-strong hover:text-ink mb-3.5 inline-flex max-w-full items-center gap-2.5 rounded-full border py-1.5 pr-3 pl-2.5 text-[13.5px] transition-colors"
    >
      <MiniLine lines={runLines(stations)} pending={pendingLine} />
      <span className="hidden truncate sm:inline">{t("summary", { tools })}</span>
      <span className="truncate sm:hidden">{t("summaryShort", { tools })}</span>
      <span className="tabular text-ink-3 text-xs">{t("seconds", { seconds })}</span>
      <ChevronDown
        aria-hidden
        className={cn("size-4 shrink-0 transition-transform", expanded && "rotate-180")}
      />
    </button>
  );
}
