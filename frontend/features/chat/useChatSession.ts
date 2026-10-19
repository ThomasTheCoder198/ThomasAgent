"use client";

import { useChat } from "@ai-sdk/react";
import { lastAssistantMessageIsCompleteWithApprovalResponses } from "ai";
import { useLocale } from "next-intl";
import { useMemo, useState } from "react";

import type { ModelChoice } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import type { ApprovalDecision } from "./components/ApprovalSign";
import type { ChatMessage } from "./contract";
import { toRunView, type RunView } from "./run-view";
import { createChatTransport, type ChatSendOptions } from "./transport";

const ALWAYS_ALLOW_REASON = "always-allow";

type Options = {
  orgId: string;
  agentId: string | undefined;
  chatId: string;
  initialMessages: ChatMessage[];
  modelChoice: ModelChoice;
  conversationPath: string;
};

/** AI SDK `useChat` wired to core's stream, plus the model choice and the approval answer. */
export function useChatSession({
  orgId,
  agentId,
  chatId,
  initialMessages,
  modelChoice,
  conversationPath,
}: Options) {
  const locale = useLocale() as Locale;
  const notify = useNotify();
  const [modelId, setModelId] = useState(modelChoice.defaultModelId);
  const transport = useMemo(() => createChatTransport(orgId, locale), [orgId, locale]);
  const chat = useChat<ChatMessage>({
    id: chatId,
    messages: initialMessages,
    transport,
    sendAutomaticallyWhen: lastAssistantMessageIsCompleteWithApprovalResponses,
    // One handler covers a dropped stream and a failed resume after an approval: both end in `status: "error"`,
    // where the thread also offers an inline retry.
    onError: notify.error,
  });
  const views = useMemo(
    () => new Map<string, RunView>(chat.messages.map((m) => [m.id, toRunView(m)])),
    [chat.messages],
  );
  // agentId only matters on the first send of a new conversation; core ignores it for a stored one (R12).
  const requestOptions = { body: { modelId, agentId } satisfies ChatSendOptions };

  function send(text: string) {
    // The URL follows the conversation without remounting the stream (Next syncs native history calls).
    if (chat.messages.length === 0) window.history.replaceState(null, "", conversationPath);
    void chat.sendMessage({ text }, requestOptions);
  }

  function decide({ approvalId, approved, always }: ApprovalDecision) {
    const reason = always ? ALWAYS_ALLOW_REASON : undefined;
    Promise.resolve(
      chat.addToolApprovalResponse({ id: approvalId, approved, reason, options: requestOptions }),
    ).catch(notify.error);
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
