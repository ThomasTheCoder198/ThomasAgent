import { randomUUID } from "node:crypto";

import { ChatPage } from "@/features/chat/ChatPage";

/** A fresh line: the id is minted here so the first message can be addressed before core stores it. */
export default function NewChatPage() {
  return <ChatPage chatId={randomUUID()} />;
}
