"use client";

import { BookOpen, Brain } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { lineIcons } from "@/components/metro/lineIcons";
import { lineBg, type Line } from "@/components/metro/lines";
import { cn } from "@/lib/cn";
import { formatSeconds } from "@/lib/format";

import { KB_TOOLS } from "../contract";
import type { Station, ToolStation } from "../run-view";
import { ApprovalSign, type ApprovalDecision } from "./ApprovalSign";
import { ReasoningBody, ToolBody, ToolLabel } from "./StationContent";
import { StationMarker } from "./StationMarker";

function stationLine(station: Station): Line {
  return station.kind === "tool" ? station.line : "model";
}

const iconClass = "text-ink-2 size-4 shrink-0";

function StationIcon({ station }: { station: Station }) {
  if (station.kind !== "tool") return <Brain aria-hidden className={iconClass} />;
  if (station.name === KB_TOOLS.readSection) return <BookOpen aria-hidden className={iconClass} />;
  const Icon = lineIcons[station.line];
  return <Icon aria-hidden className={iconClass} />;
}

function markerTone(station: Station) {
  if (station.kind !== "tool") return "default";
  if (station.state === "error") return "error";
  if (station.state === "denied") return "muted";
  if (station.state === "awaiting") return "awaiting";
  return "default";
}

function StationHeading({ station, live }: { station: Station; live: boolean }) {
  const t = useTranslations("chat");
  const locale = useLocale();
  const duration = station.kind === "status" ? undefined : station.durationMs;
  let label;
  if (station.kind === "status") label = station.label;
  else if (station.kind === "reasoning") label = live ? t("thinkingLive") : t("thinking");
  else label = <ToolLabel station={station} />;
  return (
    <div className={cn("flex items-center gap-2 text-sm leading-snug", live && "font-semibold")}>
      <StationIcon station={station} />
      <span className="min-w-0">{label}</span>
      {duration !== undefined && !live && (
        <span className="tabular text-ink-3 ml-auto shrink-0 pl-2 text-xs">
          {t("seconds", { seconds: formatSeconds(duration, locale) })}
        </span>
      )}
    </div>
  );
}

type RunLineProps = {
  stations: Station[];
  /** The id of the station that is streaming right now, if any. */
  liveId?: string;
  onDecide: (decision: ApprovalDecision) => void;
  deciding?: boolean;
};

function isLive(station: Station, liveId: string | undefined) {
  if (station.id !== liveId) return false;
  return station.kind !== "tool" || station.state === "running";
}

/** The signature move: the agent's work as a vertical line in the message gutter, one station per streamed part. */
export function RunLine({ stations, liveId, onDecide, deciding }: RunLineProps) {
  return (
    <ol className="relative my-1 mb-4 pl-[34px]">
      {stations.map((station, index) => {
        const line = stationLine(station);
        const live = isLive(station, liveId);
        const last = index === stations.length - 1;
        return (
          <li key={station.id} className={cn("animate-rise relative", last ? "pb-1" : "pb-4")}>
            {!last && (
              // The stem grows down to the next station when that station arrives.
              <span
                aria-hidden
                className={cn(
                  "animate-stem absolute top-5 -bottom-0.5 -left-6 w-[3px] origin-top rounded-sm",
                  lineBg[line],
                )}
              />
            )}
            <span className={cn("absolute", live ? "-top-[3px] -left-9" : "top-0.5 -left-[31px]")}>
              <StationMarker line={line} live={live} tone={markerTone(station)} />
            </span>
            <StationHeading station={station} live={live} />
            {station.kind === "reasoning" && station.text && <ReasoningBody station={{ ...station, live }} />}
            {station.kind === "tool" && station.state === "awaiting" && (
              <ApprovalSign station={station as ToolStation} onDecide={onDecide} disabled={deciding} />
            )}
            {station.kind === "tool" && station.state !== "awaiting" && <ToolBody station={station} />}
          </li>
        );
      })}
    </ol>
  );
}
