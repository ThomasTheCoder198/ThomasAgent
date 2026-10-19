"use client";

import { Bot, ScanSearch } from "lucide-react";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { MenuButton } from "@/features/shell/MenuButton";
import { pageOwnsHeader } from "@/features/shell/shell-menu";

type Props = { title: string; agentName?: string; runId?: string; onInspect?: () => void };

/**
 * The conversation's bar: title, Agent, run id and the inspector. Below md it carries the shell's menu
 * button only when the shell leaves the mobile bar to this page, so phones never get two bars.
 */
export function ChatHeader({ title, agentName, runId, onInspect }: Props) {
  const t = useTranslations("chat");
  const ownsMobileBar = pageOwnsHeader(usePathname() ?? "");
  return (
    <header className="border-border bg-background flex h-14 shrink-0 items-center gap-3 border-b px-4 md:px-6">
      {ownsMobileBar && <MenuButton />}
      <h1 className="min-w-0 truncate text-base leading-snug font-semibold md:text-lg">{title}</h1>
      {agentName && (
        <Badge variant="outline" className="hidden sm:inline-flex">
          <Bot aria-hidden />
          {agentName}
        </Badge>
      )}
      {runId && <span className="tabular text-muted-foreground shrink-0 text-xs">{runId}</span>}
      {onInspect && (
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("inspect")}
          title={t("inspect")}
          onClick={onInspect}
          className="ml-auto"
        >
          <ScanSearch aria-hidden />
        </Button>
      )}
    </header>
  );
}
