"use client";

import { Bot } from "lucide-react";
import { useTranslations } from "next-intl";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import type { Agent } from "@/lib/core/types";

/** With fewer Agents than this there is nothing to choose. */
const MIN_AGENTS_TO_CHOOSE = 2;

type Props = { agents: Agent[]; agentId: string | undefined; onChange: (id: string) => void };

/**
 * Which Agent answers a new chat (R12). Only offered before the first message: a conversation stays bound to its
 * Agent, and with a single Agent there is nothing to choose.
 */
export function AgentPicker({ agents, agentId, onChange }: Props) {
  const t = useTranslations("chat");
  if (agents.length < MIN_AGENTS_TO_CHOOSE) return null;
  return (
    <Select value={agentId} onValueChange={onChange}>
      <SelectTrigger
        aria-label={t("agent")}
        className="h-11 w-full max-w-sm *:data-[slot=select-value]:flex-1 *:data-[slot=select-value]:text-left md:h-10"
      >
        <Bot aria-hidden className="text-muted-foreground" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {agents.map((agent) => (
          <SelectItem key={agent.id} value={agent.id}>
            {agent.name}{" "}
            <span className="text-muted-foreground ml-2 text-xs">
              {agent.kbMissing ? t("kbDeletedShort") : agent.kbName}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
