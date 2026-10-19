import { DefaultChatTransport } from "ai";

import type { Locale } from "@/i18n/config";
import { mutationHeaders } from "@/lib/api/client";

import { CHAT_PATHS, type ChatMessage, type ChatRequestBody } from "./contract";

/** Per-request options passed to sendMessage / regenerate (`body`). */
export type ChatSendOptions = { modelId?: string; agentId?: string };

type BodyArgs = {
  id: string;
  trigger: ChatRequestBody["trigger"];
  messageId: string | undefined;
  messages: ChatMessage[];
  options: Record<string, unknown> | undefined;
};

const text = (value: unknown) => (typeof value === "string" ? value : undefined);

/** Only the newest message goes up; core rebuilds history itself (spec §7.1) and binds a new chat to `agentId` (R12). */
export function toChatBody({ id, trigger, messageId, messages, options }: BodyArgs): ChatRequestBody {
  const message = messages.at(-1);
  if (!message) throw new Error("chat transport: nothing to send");
  return {
    id,
    trigger,
    messageId,
    message,
    modelId: text(options?.modelId),
    agentId: text(options?.agentId),
  };
}

/** Real HTTP transport to core's chat stream, through the same-origin `/api/v1` rewrite. */
export function createChatTransport(orgId: string, locale: Locale) {
  return new DefaultChatTransport<ChatMessage>({
    api: CHAT_PATHS.chat(orgId),
    credentials: "same-origin",
    headers: () => mutationHeaders(locale),
    prepareSendMessagesRequest: ({ id, messages, trigger, messageId, body }) => ({
      body: toChatBody({
        id,
        trigger,
        messageId,
        messages,
        options: body as Record<string, unknown> | undefined,
      }),
    }),
  });
}
