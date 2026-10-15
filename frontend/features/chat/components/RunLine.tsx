"use client";

import { BookOpen, Brain } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { lineIcons } from "@/components/lineIcons";
import { lineBg, type Line } from "@/components/lines";
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

const iconClass = "text-muted-foreground size-4 shrink-0";

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
    <div className={cn("flex min-h-7 items-center gap-2 text-sm", live && "font-semibold")}>
      <StationIcon station={station} />
      <span className="min-w-0">{label}</span>
      {duration !== undefined && !live && (
        <span className="tabular text-muted-foreground ml-auto shrink-0 pl-2 text-xs">
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

/**
 * The signature move: the agent's work as a vertical line in the message gutter, one station per streamed
 * part. Each station is its own two-column grid (marker + stem | content), so the stem always reaches the
 * next marker however tall the content grows.
 */
export function RunLine({ stations, liveId, onDecide, deciding }: RunLineProps) {
  return (
    <ol className="mb-4">
      {stations.map((station, index) => {
        const line = stationLine(station);
        const live = isLive(station, liveId);
        const last = index === stations.length - 1;
        return (
          <li key={station.id} className="animate-rise grid grid-cols-[1.75rem_1fr] gap-x-3">
            <div aria-hidden className="flex flex-col items-center">
              <span className="grid h-7 place-items-center">
                <StationMarker line={line} live={live} tone={markerTone(station)} />
              </span>
              {!last && (
                // The stem grows down to the next station when that station arrives; the negative margins tuck
                // it under this roundel and into the next slot so the line reads as one.
                <span
                  className={cn("animate-stem -my-1 w-[3px] flex-1 origin-top rounded-sm", lineBg[line])}
                />
              )}
            </div>
            <div className={cn("min-w-0", !last && "pb-4")}>
              <StationHeading station={station} live={live} />
              {station.kind === "reasoning" && station.text && (
                <ReasoningBody station={{ ...station, live }} />
              )}
              {station.kind === "tool" && station.state === "awaiting" && (
                <ApprovalSign station={station as ToolStation} onDecide={onDecide} disabled={deciding} />
              )}
              {station.kind === "tool" && station.state !== "awaiting" && <ToolBody station={station} />}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
