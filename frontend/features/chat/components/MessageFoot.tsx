"use client";

import { Check, Copy, ScanSearch } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { IconButton } from "@/components/ui/Button";
import { formatCost, formatSeconds, formatTokens } from "@/lib/format";

import type { RunUsage } from "../contract";

const COPIED_RESET_MS = 1600;

type Props = { text: string; usage: RunUsage | undefined; onInspect: () => void };

/** Actions on a finished answer and its measurements: run id, tokens, cost, time to first token. */
export function MessageFoot({ text, usage, onInspect }: Props) {
  const t = useTranslations("chat");
  const locale = useLocale();
  const [copied, setCopied] = useState(false);
  async function copy() {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), COPIED_RESET_MS);
  }
  return (
    <div className="text-ink-3 mt-1.5 flex flex-wrap items-center gap-0.5">
      <IconButton label={copied ? t("copied") : t("copy")} onClick={copy}>
        {copied ? <Check aria-hidden className="size-4" /> : <Copy aria-hidden className="size-4" />}
      </IconButton>
      <IconButton label={t("inspect")} onClick={onInspect}>
        <ScanSearch aria-hidden className="size-4" />
      </IconButton>
      {usage && (
        <p className="tabular ml-2.5 flex flex-wrap gap-x-3.5 text-xs">
          <span>{usage.runId}</span>
          <span>{t("tokens", { count: formatTokens(usage.inputTokens + usage.outputTokens, locale) })}</span>
          <span>{formatCost(usage.costUsd, locale)}</span>
          <span>{t("ttft", { seconds: formatSeconds(usage.ttftMs, locale) })}</span>
        </p>
      )}
    </div>
  );
}
