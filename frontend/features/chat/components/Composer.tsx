"use client";

import { ArrowUp, Paperclip, Square } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState, type FormEvent, type KeyboardEvent } from "react";

import { lineBg } from "@/components/lines";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import { Textarea } from "@/components/ui/Textarea";
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

/** The composer's pills share one 32px height; the Select trigger and the badges must line up. */
const PILL = "h-8 px-3 text-[0.8125rem] font-normal";

function ModelPicker({
  models,
  modelId,
  onModelChange,
}: Pick<Props, "models" | "modelId" | "onModelChange">) {
  const t = useTranslations("chat");
  if (models.length === 0) return null;
  return (
    <Select value={modelId} onValueChange={onModelChange}>
      <SelectTrigger
        size="sm"
        aria-label={t("model")}
        className={cn(
          PILL,
          "border-border text-muted-foreground max-w-48 min-w-0 rounded-full bg-transparent",
        )}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {models.map((m) => (
          <SelectItem key={m.id} value={m.id}>
            {m.displayName}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function ScopePills({ scope }: { scope: ChatScope }) {
  const t = useTranslations("chat");
  return (
    <>
      <Badge variant="outline" className={cn(PILL, "hidden max-w-44 sm:inline-flex")} title={scope.kb.name}>
        <span className="bg-kb size-2 shrink-0 rounded-full" aria-hidden />
        <span className="truncate">{scope.kb.name}</span>
      </Badge>
      <Badge
        variant="outline"
        className={cn(PILL, "hidden md:inline-flex")}
        title={scope.toolSources.map((s) => s.name).join(", ")}
      >
        <span className="flex items-center gap-1" aria-hidden>
          {scope.toolSources.map((source) => (
            <span
              key={`${source.line}-${source.name}`}
              className={cn("size-2 rounded-full", lineBg[source.line])}
            />
          ))}
        </span>
        {t("toolScope", { count: scope.toolSources.length })}
      </Badge>
    </>
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
      className="tabular text-muted-foreground ml-auto hidden items-center gap-2 text-xs sm:flex"
    >
      <span className="bg-border h-1 w-16 overflow-hidden rounded-full">
        <span
          className="bg-foreground block h-full rounded-full"
          style={{ width: `${Math.min(ratio, 1) * PERCENT}%` }}
        />
      </span>
      {percent}%
    </div>
  );
}

/** The one surface with a sticker: a card with an ink outline, whatever the thread above it holds. */
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
      className="border-outline bg-card shadow-sticker-lg focus-within:ring-ring ring-offset-background rounded-lg border-[1.5px] focus-within:ring-2 focus-within:ring-offset-2"
    >
      <Textarea
        value={text}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={onKeyDown}
        rows={1}
        aria-label={t("inputLabel")}
        placeholder={t("placeholder")}
        className="max-h-48 min-h-12 resize-none rounded-none border-0 bg-transparent px-4 pt-4 pb-2 outline-none focus-visible:outline-none"
      />
      <div className="flex items-center gap-2 px-3 pb-3">
        <Button variant="ghost" size="icon" aria-label={t("attachSoon")} title={t("attachSoon")} disabled>
          <Paperclip aria-hidden />
        </Button>
        <ModelPicker models={models} modelId={modelId} onModelChange={onModelChange} />
        {scope && <ScopePills scope={scope} />}
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
            <ArrowUp aria-hidden />
          </Button>
        )}
      </div>
    </form>
  );
}
