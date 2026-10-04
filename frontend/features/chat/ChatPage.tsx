import { getTranslations } from "next-intl/server";

import { ChatSurface } from "./ChatSurface";
import { getConversation, getModelChoice, getScope } from "./server";

/** Server half of a chat route: loads what core knows about the conversation, then hands over to the stream. */
export async function ChatPage({ chatId }: { chatId: string }) {
  const [t, conversation, scope, modelChoice] = await Promise.all([
    getTranslations("shell"),
    getConversation(chatId),
    getScope(),
    getModelChoice(),
  ]);
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
