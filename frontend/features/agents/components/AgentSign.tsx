"use client";

import { ArrowLeft, LibraryBig } from "lucide-react";
import Link from "next/link";
import { useTranslations } from "next-intl";

import { LineBadge } from "@/components/metro/LineBadge";
import { LineDots } from "@/components/metro/LineDots";
import { RouteStrip, type RouteStop } from "@/components/metro/RouteStrip";
import { MenuButton } from "@/features/shell/MenuButton";

import type { AgentToolSource } from "../contract";

type Props = {
  name: string;
  description: string;
  kbName: string;
  sources: AgentToolSource[];
  toolCount: number;
  model: string;
};

/** The Agent's platform sign: its name at display size and its route — model, KB, then each tool line. */
export function AgentSign({ name, description, kbName, sources, toolCount, model }: Props) {
  const t = useTranslations("agents");
  const stops: RouteStop[] = [
    { line: "model", label: model || "Model" },
    { line: "kb", label: kbName, short: "KB" },
    ...sources.map((s) => ({ line: s.line, label: s.name })),
  ];
  return (
    <header className="bg-sign-band text-sign-ink border-sign-rule dark:border-b">
      <div className="mx-auto w-full max-w-272 px-4 pt-4 pb-6 md:px-8 md:pt-6 md:pb-7">
        <div className="flex items-center gap-2">
          <MenuButton tone="sign" />
          <Link
            href="/agents"
            className="text-sign-ink-2 hover:text-sign-ink inline-flex items-center gap-1.5 text-xs font-medium transition-colors"
          >
            <ArrowLeft aria-hidden className="size-3.5" />
            {t("back")}
          </Link>
        </div>
        <h1 className="animate-rise mt-4 truncate text-[clamp(1.75rem,3vw,2.6rem)] leading-tight font-bold tracking-[-0.025em]">
          {name}
        </h1>
        {description && <p className="text-sign-ink-2 mt-1.5 max-w-[70ch] truncate">{description}</p>}
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
          <LineBadge line="kb" icon={LibraryBig}>
            {kbName}
          </LineBadge>
          <span className="text-sign-ink-2 inline-flex items-center gap-2">
            <LineDots lines={sources.map((s) => s.line)} size="md" />
            {t("toolCount", { count: toolCount })}
          </span>
          {model && <span className="tabular text-sign-ink-2 text-xs">{model}</span>}
        </div>
        <div className="animate-draw-x mt-6 max-w-xl">
          <RouteStrip stops={stops} />
        </div>
      </div>
    </header>
  );
}
