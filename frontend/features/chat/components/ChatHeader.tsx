"use client";

import { Folder, ScanSearch } from "lucide-react";
import { useTranslations } from "next-intl";

import { IconButton } from "@/components/ui/Button";
import { MenuButton } from "@/features/shell/MenuButton";

type Props = { title: string; folder?: string; runId?: string; onInspect?: () => void };

/** The conversation's sign: on phones it is the only bar (menu, title, run id, inspector). */
export function ChatHeader({ title, folder, runId, onInspect }: Props) {
  const t = useTranslations("chat");
  return (
    <header className="border-rule bg-ground flex h-13 shrink-0 items-center gap-2.5 border-b px-3 md:h-14.5 md:gap-3 md:px-7">
      <MenuButton />
      <h1 className="min-w-0 truncate text-[16px] font-bold tracking-[-0.012em] md:text-[19px]">{title}</h1>
      {folder && (
        <span className="border-rule bg-paper text-ink-2 hidden shrink-0 items-center gap-1.5 rounded-full border px-2.25 py-0.75 text-[12.5px] sm:inline-flex">
          <Folder aria-hidden className="size-3.5" />
          {folder}
        </span>
      )}
      {runId && <span className="tabular text-ink-3 shrink-0 text-[11px] md:text-xs">{runId}</span>}
      {onInspect && (
        <IconButton label={t("inspect")} onClick={onInspect} className="ml-auto">
          <ScanSearch aria-hidden className="size-4" />
        </IconButton>
      )}
    </header>
  );
}
