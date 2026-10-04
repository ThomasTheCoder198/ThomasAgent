"use client";

import { Plug } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import type { ToolStation } from "../run-view";
import { LineBadge } from "./LineBadge";

export type ApprovalDecision = { approvalId: string; approved: boolean; always?: boolean };

type Props = { station: ToolStation; onDecide: (decision: ApprovalDecision) => void; disabled?: boolean };

function formatArg(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "object" && value !== null) return JSON.stringify(value);
  return String(value);
}

/**
 * A notice board in the tool's line colour: the real arguments the model wants to send, and the
 * owner's three answers. Risky tools never run until one is chosen (spec §7.2, policy `ask`).
 */
export function ApprovalSign({ station, onDecide, disabled }: Props) {
  const t = useTranslations("chat");
  const approvalId = station.approvalId ?? "";
  const decide = (approved: boolean, always = false) => onDecide({ approvalId, approved, always });
  return (
    <section
      aria-label={t("approvalTitle")}
      className={cn("border-mcp-rule bg-mcp-tint mt-2 max-w-[62ch] rounded-xl border px-4 py-3.5")}
    >
      <header className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5 text-sm">
        <LineBadge line={station.line} icon={Plug}>
          {station.source}
        </LineBadge>
        <b className="font-semibold">{station.title}</b>
        <span className="text-mcp-ink-2 ml-auto text-xs font-medium">{t("approvalTitle")}</span>
      </header>
      <dl className="tabular text-mcp-ink my-3 grid grid-cols-[auto_1fr] gap-x-3 text-[12.5px] leading-[1.7]">
        {Object.entries(station.input).map(([key, value]) => (
          <div key={key} className="contents">
            <dt className="text-mcp-ink-2">{key}</dt>
            <dd className="min-w-0 break-words">{formatArg(value)}</dd>
          </div>
        ))}
      </dl>
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="line" disabled={disabled} onClick={() => decide(true)}>
          {t("approve")}
        </Button>
        <Button variant="outline" disabled={disabled} onClick={() => decide(false)}>
          {t("deny")}
        </Button>
        <Button
          variant="ghost"
          disabled={disabled}
          onClick={() => decide(true, true)}
          className="ml-auto font-medium"
        >
          {t("alwaysAllow")}
        </Button>
      </div>
    </section>
  );
}
