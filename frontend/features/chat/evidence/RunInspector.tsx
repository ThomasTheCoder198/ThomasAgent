"use client";

import { useLocale, useTranslations } from "next-intl";

import { formatCost, formatSeconds, formatTokens } from "@/lib/format";

import type { RunUsage } from "../contract";
import { MiniLine } from "../components/MiniLine";
import { runLines, totalDurationMs, type Station } from "../run-view";

type Props = { stations: Station[]; usage: RunUsage | undefined; runId: string | undefined };

function stationName(station: Station, thought: string) {
  if (station.kind === "status") return station.label;
  if (station.kind === "reasoning") return thought;
  return `${station.source} · ${station.title}`;
}

/** The whole route of one run with per-station timing; token and cost totals come from `data-usage`. */
export function RunInspector({ stations, usage, runId }: Props) {
  const t = useTranslations("evidence");
  const tChat = useTranslations("chat");
  const locale = useLocale();
  if (stations.length === 0) return <p className="text-ink-2 p-5 text-sm">{t("noRun")}</p>;
  const seconds = (ms: number | undefined) =>
    ms === undefined ? "–" : tChat("seconds", { seconds: formatSeconds(ms, locale) });
  return (
    <div className="p-4.5">
      <div className="mb-3.5 flex items-center gap-3">
        <MiniLine lines={runLines(stations)} />
        {runId && <span className="tabular text-ink-3 ml-auto text-xs">{runId}</span>}
      </div>
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-ink-3 text-left text-xs">
            <th className="pb-2 font-medium">{t("station")}</th>
            <th className="pb-2 text-right font-medium">{t("duration")}</th>
          </tr>
        </thead>
        <tbody>
          {stations.map((station) => (
            <tr key={station.id} className="border-rule-2 border-t">
              <td className="py-2 pr-3">{stationName(station, tChat("thinking"))}</td>
              <td className="tabular text-ink-2 py-2 text-right text-xs">
                {seconds(station.kind === "status" ? undefined : station.durationMs)}
              </td>
            </tr>
          ))}
        </tbody>
        <tfoot className="tabular text-xs">
          <tr className="border-rule border-t font-semibold">
            <td className="pt-2.5">{t("total")}</td>
            <td className="pt-2.5 text-right">{seconds(usage?.durationMs ?? totalDurationMs(stations))}</td>
          </tr>
          {usage && (
            <tr className="text-ink-2">
              <td className="pt-1">
                {tChat("tokens", { count: formatTokens(usage.inputTokens + usage.outputTokens, locale) })}
              </td>
              <td className="pt-1 text-right">
                {t("cost")} {formatCost(usage.costUsd, locale)}
              </td>
            </tr>
          )}
        </tfoot>
      </table>
    </div>
  );
}
