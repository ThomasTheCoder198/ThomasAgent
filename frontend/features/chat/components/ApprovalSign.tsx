"use client";

import { useTranslations } from "next-intl";

import { lineIcons } from "@/components/lineIcons";
import { lineBg } from "@/components/lines";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import type { ToolStation } from "../run-view";
import { SourceBadge } from "./SourceBadge";

export type ApprovalDecision = { approvalId: string; approved: boolean; always?: boolean };

type Props = { station: ToolStation; onDecide: (decision: ApprovalDecision) => void; disabled?: boolean };

function formatArg(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "object" && value !== null) return JSON.stringify(value);
  return String(value);
}

/**
 * A card with the tool's own line colour as its left rule: the real arguments the model wants to send, and
 * the owner's three answers. Risky tools never run until one is chosen (spec §7.2, policy `ask`).
 */
export function ApprovalSign({ station, onDecide, disabled }: Props) {
  const t = useTranslations("chat");
  const approvalId = station.approvalId ?? "";
  const decide = (approved: boolean, always = false) => onDecide({ approvalId, approved, always });
  return (
    <section
      aria-label={t("approvalTitle")}
      className="border-border bg-card animate-rise relative mt-2 max-w-xl overflow-hidden rounded-sm border py-4 pr-4 pl-5"
    >
      <span aria-hidden className={cn("absolute inset-y-0 left-0 w-[3px]", lineBg[station.line])} />
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <SourceBadge line={station.line} icon={lineIcons[station.line]}>
          {station.source}
        </SourceBadge>
        <b className="font-semibold">{station.title}</b>
        <span className="text-muted-foreground ml-auto text-xs">{t("approvalTitle")}</span>
      </header>
      <dl className="tabular my-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs leading-relaxed">
        {Object.entries(station.input).map(([key, value]) => (
          <div key={key} className="contents">
            <dt className="text-muted-foreground">{key}</dt>
            <dd className="text-foreground min-w-0 break-words">{formatArg(value)}</dd>
          </div>
        ))}
      </dl>
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={disabled} onClick={() => decide(true)}>
          {t("approve")}
        </Button>
        <Button variant="outline" disabled={disabled} onClick={() => decide(false)}>
          {t("deny")}
        </Button>
        <Button
          variant="ghost"
          disabled={disabled}
          onClick={() => decide(true, true)}
          className="font-medium sm:ml-auto"
        >
          {t("alwaysAllow")}
        </Button>
      </div>
    </section>
  );
}
