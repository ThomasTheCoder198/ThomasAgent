"use client";

import { Check, Copy, RotateCcw, ScanSearch } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { formatCost, formatSeconds, formatTokens } from "@/lib/format";
import { useNotify } from "@/lib/useNotify";

import type { RunUsage } from "../contract";

const COPIED_RESET_MS = 1600;

type Props = {
  text: string;
  usage: RunUsage | undefined;
  onInspect: () => void;
  /** Only the newest answer can be asked again. */
  onRegenerate?: () => void;
};

/** Actions on a finished answer and its measurements: run id, tokens, cost, time to first token. */
export function MessageFoot({ text, usage, onInspect, onRegenerate }: Props) {
  const t = useTranslations("chat");
  const locale = useLocale();
  const notify = useNotify();
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
    } catch (err) {
      notify.error(err);
      return;
    }
    setCopied(true);
    setTimeout(() => setCopied(false), COPIED_RESET_MS);
  }
  return (
    <div className="text-muted-foreground animate-fade mt-2 flex flex-wrap items-center gap-x-1 gap-y-2">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={copied ? t("copied") : t("copy")}
        title={copied ? t("copied") : t("copy")}
        onClick={copy}
      >
        {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
      </Button>
      {onRegenerate && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t("regenerate")}
          title={t("regenerate")}
          onClick={onRegenerate}
        >
          <RotateCcw aria-hidden />
        </Button>
      )}
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t("inspect")}
        title={t("inspect")}
        onClick={onInspect}
      >
        <ScanSearch aria-hidden />
      </Button>
      {usage && (
        <p className="tabular ml-2 flex flex-wrap gap-x-4 text-xs">
          <span>{usage.runId}</span>
          <span>{t("tokens", { count: formatTokens(usage.inputTokens + usage.outputTokens, locale) })}</span>
          <span>{formatCost(usage.costUsd, locale)}</span>
          <span>{t("ttft", { seconds: formatSeconds(usage.ttftMs, locale) })}</span>
        </p>
      )}
    </div>
  );
}
