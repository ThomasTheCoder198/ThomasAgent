"use client";

import { useChat } from "@ai-sdk/react";
import { lastAssistantMessageIsCompleteWithApprovalResponses } from "ai";
import { useLocale } from "next-intl";
import { useMemo, useState } from "react";

import type { ModelChoice } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";

import type { ApprovalDecision } from "./components/ApprovalSign";
import type { ChatMessage } from "./contract";
import { toRunView, type RunView } from "./run-view";
import { createChatTransport } from "./transport";

const ALWAYS_ALLOW_REASON = "always-allow";

type Options = {
  chatId: string;
  initialMessages: ChatMessage[];
  modelChoice: ModelChoice;
  conversationPath: string;
};

/** AI SDK `useChat` wired to core's stream, plus the model choice and the approval answer. */
export function useChatSession({ chatId, initialMessages, modelChoice, conversationPath }: Options) {
  const locale = useLocale() as Locale;
  const [modelId, setModelId] = useState(modelChoice.defaultModelId);
  const transport = useMemo(() => createChatTransport(locale), [locale]);
  const chat = useChat<ChatMessage>({
    id: chatId,
    messages: initialMessages,
    transport,
    sendAutomaticallyWhen: lastAssistantMessageIsCompleteWithApprovalResponses,
  });
  const views = useMemo(
    () => new Map<string, RunView>(chat.messages.map((m) => [m.id, toRunView(m)])),
    [chat.messages],
  );
  const requestOptions = { body: { modelId } };

  function send(text: string) {
    // The URL follows the conversation without remounting the stream (Next syncs native history calls).
    if (chat.messages.length === 0) window.history.replaceState(null, "", conversationPath);
    void chat.sendMessage({ text }, requestOptions);
  }

  function decide({ approvalId, approved, always }: ApprovalDecision) {
    const reason = always ? ALWAYS_ALLOW_REASON : undefined;
    void chat.addToolApprovalResponse({ id: approvalId, approved, reason, options: requestOptions });
  }

  return {
    messages: chat.messages,
    status: chat.status,
    busy: chat.status === "submitted" || chat.status === "streaming",
    views,
    modelId,
    setModelId,
    send,
    decide,
    stop: () => void chat.stop(),
    retry: () => void chat.regenerate(requestOptions),
  };
}

/** Fraction of the chosen model's context window the last finished run used. */
export function contextRatio(
  view: RunView | undefined,
  modelChoice: ModelChoice,
  modelId: string | undefined,
): number {
  const window = modelChoice.models.find((m) => m.id === modelId)?.contextWindow ?? 0;
  if (!view?.usage || window <= 0) return 0;
  return (view.usage.inputTokens + view.usage.outputTokens) / window;
}
