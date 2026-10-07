import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";

import { ChatSurface } from "./ChatSurface";
import type { ConversationDetail } from "./contract";
import { getConversation, getModelChoice, getScope } from "./server";

function requireConversation(conversation: ConversationDetail | null, required: boolean) {
  if (required && !conversation) notFound();
}

/** Server half of a chat route: loads what core knows about the conversation, then hands over to the stream. */
export async function ChatPage({
  chatId,
  requireExisting = false,
}: {
  chatId: string;
  requireExisting?: boolean;
}) {
  const [t, conversation, scope, modelChoice] = await Promise.all([
    getTranslations("shell"),
    getConversation(chatId),
    getScope(),
    getModelChoice(),
  ]);
  requireConversation(conversation, requireExisting);
  return (
    <ChatSurface
      key={chatId}
      chatId={chatId}
      initialMessages={conversation?.messages ?? []}
      title={conversation?.title ?? t("newChat")}
      folder={conversation?.folder}
      scope={conversation?.scope ?? scope}
      modelChoice={modelChoice}
      conversationPath={`/chat/${encodeURIComponent(chatId)}`}
    />
  );
}
