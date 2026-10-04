import { DefaultChatTransport } from "ai";

import type { Locale } from "@/i18n/config";
import { mutationHeaders } from "@/lib/api/client";

import { CHAT_PATHS, type ChatMessage, type ChatRequestBody } from "./contract";

/**
 * Real HTTP transport to core's chat stream (through the same-origin `/api/v1` rewrite). It sends only the
 * newest message plus approval answers; core rebuilds history from its database (spec §7.1).
 */
export function createChatTransport(locale: Locale) {
  return new DefaultChatTransport<ChatMessage>({
    api: CHAT_PATHS.chat,
    credentials: "same-origin",
    headers: () => mutationHeaders(locale),
    // `body` carries per-request options (the chosen model) passed to sendMessage / addToolApprovalResponse.
    prepareSendMessagesRequest: ({ id, messages, trigger, messageId, body: options }) => {
      const message = messages.at(-1);
      if (!message) throw new Error("chat transport: nothing to send");
      const modelId = typeof options?.modelId === "string" ? options.modelId : undefined;
      const body: ChatRequestBody = { id, trigger, messageId, message, modelId };
      return { body };
    },
  });
}
