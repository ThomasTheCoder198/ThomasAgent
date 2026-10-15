"use client";

import { ArrowUp, ChevronDown, Paperclip, Square } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState, type FormEvent, type KeyboardEvent } from "react";

import { LineDots } from "@/components/metro/LineDots";
import { Button } from "@/components/ui/Button";
import type { Model } from "@/features/registry/types";
import { cn } from "@/lib/cn";
import { formatPercent, PERCENT } from "@/lib/format";

import type { ChatScope } from "../contract";

type Props = {
  busy: boolean;
  onSend: (text: string) => void;
  onStop: () => void;
  models: Model[];
  modelId: string | undefined;
  onModelChange: (id: string) => void;
  scope: ChatScope | null;
  contextRatio: number;
};

const pill =
  "border-border bg-secondary text-muted-foreground inline-flex items-center gap-1.75 rounded-full border px-2.5 py-1.25 text-[12.5px] whitespace-nowrap";

function ModelPicker({
  models,
  modelId,
  onModelChange,
}: Pick<Props, "models" | "modelId" | "onModelChange">) {
  const t = useTranslations("chat");
  if (models.length === 0) return null;
  return (
    <label className={cn(pill, "focus-within:outline-ring relative pr-7 focus-within:outline-2")}>
      <span className="bg-model size-2 rounded-full" aria-hidden />
      <span className="sr-only">{t("model")}</span>
      <select
        value={modelId}
        onChange={(event) => onModelChange(event.target.value)}
        className="text-muted-foreground max-w-36 cursor-pointer appearance-none truncate bg-transparent outline-none"
      >
        {models.map((m) => (
          <option key={m.id} value={m.id}>
            {m.displayName}
          </option>
        ))}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute right-2 size-3.5" />
    </label>
  );
}

function ContextMeter({ ratio }: { ratio: number }) {
  const t = useTranslations("chat");
  const locale = useLocale();
  const percent = formatPercent(ratio, locale);
  return (
    <div
      role="meter"
      aria-valuemin={0}
      aria-valuemax={1}
      aria-valuenow={ratio}
      aria-label={t("contextMeter", { percent })}
      title={t("contextMeter", { percent })}
      className="tabular text-muted-foreground ml-auto hidden items-center gap-2 text-[11px] sm:flex"
    >
      <span className="bg-border h-1.25 w-18.5 overflow-hidden rounded-full">
        <span
          className="bg-foreground block h-full rounded-full"
          style={{ width: `${Math.min(ratio, 1) * PERCENT}%` }}
        />
      </span>
      {percent}%
    </div>
  );
}

export function Composer({
  busy,
  onSend,
  onStop,
  models,
  modelId,
  onModelChange,
  scope,
  contextRatio,
}: Props) {
  const t = useTranslations("chat");
  const [text, setText] = useState("");
  const canSend = text.trim().length > 0 && !busy;

  function submit(event?: FormEvent) {
    event?.preventDefault();
    if (!canSend) return;
    onSend(text.trim());
    setText("");
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    // Vietnamese IMEs (Telex, VNI) commit with Enter; never send half-composed text.
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) submit(event);
  }

  return (
    <form
      onSubmit={submit}
      className="border-input bg-card focus-within:border-foreground rounded-2xl border py-3 pr-3 pl-4 transition-colors"
    >
      <textarea
        value={text}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={onKeyDown}
        rows={1}
        aria-label={t("inputLabel")}
        placeholder={t("placeholder")}
        className="text-foreground block field-sizing-content max-h-48 min-h-6.5 w-full resize-none bg-transparent text-[15px] outline-none focus-visible:outline-none"
      />
      <div className="mt-2 flex items-center gap-1.5">
        <Button variant="ghost" size="icon" aria-label={t("attachSoon")} title={t("attachSoon")} disabled>
          <Paperclip aria-hidden className="size-4" />
        </Button>
        <ModelPicker models={models} modelId={modelId} onModelChange={onModelChange} />
        {scope && (
          <>
            <span className={cn(pill, "hidden max-w-44 sm:inline-flex")} title={scope.kb.name}>
              <span className="bg-kb size-2 shrink-0 rounded-full" aria-hidden />
              <span className="truncate">{scope.kb.name}</span>
            </span>
            <span
              className={cn(pill, "hidden md:inline-flex")}
              title={scope.toolSources.map((s) => s.name).join(", ")}
            >
              <LineDots lines={scope.toolSources.map((s) => s.line)} size="md" />
              {t("toolScope", { count: scope.toolSources.length })}
            </span>
          </>
        )}
        <ContextMeter ratio={contextRatio} />
        {busy ? (
          <Button
            size="icon"
            aria-label={t("stop")}
            title={t("stop")}
            onClick={onStop}
            className="ml-auto rounded-full sm:ml-0"
          >
            <Square aria-hidden className="size-3.5 fill-current" />
          </Button>
        ) : (
          <Button
            type="submit"
            size="icon"
            aria-label={t("send")}
            title={t("send")}
            disabled={!canSend}
            className="ml-auto rounded-full sm:ml-0"
          >
            <ArrowUp aria-hidden className="size-4.5" />
          </Button>
        )}
      </div>
    </form>
  );
}
