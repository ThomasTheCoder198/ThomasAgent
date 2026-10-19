import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";

import { listAgents } from "@/features/agents/server";
import { ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { NoOrgState } from "@/features/orgs/NoOrgState";
import { getCurrentOrg } from "@/features/orgs/server";
import type { Agent } from "@/lib/core/types";

import { ChatSurface } from "./ChatSurface";
import type { ConversationDetail } from "./contract";
import { scopeOfAgent } from "./scope";
import { getConversation, getModelChoice } from "./server";

/**
 * What the surface starts from. A stored conversation is bound to its Agent (R12) and offers no Agent picker; a new
 * one starts on the org's first Agent.
 */
function startOf(conversation: ConversationDetail | null, agents: Agent[], newTitle: string) {
  if (conversation) {
    return {
      initialMessages: conversation.messages,
      title: conversation.title,
      scope: conversation.scope,
      agents: [],
    };
  }
  return { initialMessages: [], title: newTitle, scope: agents[0] ? scopeOfAgent(agents[0]) : null, agents };
}

/** Server half of a chat route: resolves the org, loads what core knows, then hands over to the stream. */
export async function ChatPage({
  chatId,
  requireExisting = false,
}: {
  chatId: string;
  requireExisting?: boolean;
}) {
  const { org, failed, requestId } = await getCurrentOrg();
  if (failed) return <ServiceUnavailable requestId={requestId} />; // a 5xx is not "no organization" (frontend rule 9)
  if (!org) return <NoOrgState />;
  const [t, conversation, agents, modelChoice] = await Promise.all([
    getTranslations("shell"),
    getConversation(org.id, chatId),
    listAgents(org.id),
    getModelChoice(org.id),
  ]);
  if (requireExisting && !conversation) notFound();
  if (!agents.ok) return <ServiceUnavailable requestId={agents.requestId} />;
  const start = startOf(conversation, agents.data, t("newChat"));
  return (
    <ChatSurface
      key={chatId}
      orgId={org.id}
      chatId={chatId}
      {...start}
      modelChoice={modelChoice}
      conversationPath={`/chat/${encodeURIComponent(chatId)}`}
    />
  );
}
