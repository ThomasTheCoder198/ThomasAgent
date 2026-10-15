"use client";

import { RotateCcw } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { cn } from "@/lib/cn";
import type { ModelChoice } from "@/features/registry/types";

import { AssistantMessage, PendingReply, UserMessage } from "./components/AssistantMessage";
import { ChatHeader } from "./components/ChatHeader";
import { HeroIntro, HeroSources } from "./components/ChatHero";
import { Composer } from "./components/Composer";
import type { ChatMessage, ChatScope } from "./contract";
import { EvidencePanel } from "./evidence/EvidencePanel";
import type { RunView } from "./run-view";
import { contextRatio, useChatSession } from "./useChatSession";
import { useEvidence } from "./useEvidence";
import { useStickToBottom } from "./useStickToBottom";

export type ChatSurfaceProps = {
  chatId: string;
  initialMessages: ChatMessage[];
  title: string;
  folder?: string;
  scope: ChatScope | null;
  modelChoice: ModelChoice;
  /** The route the conversation gets once its first message is sent. */
  conversationPath: string;
};

/** Thread, hero and composer share one centred 768px column; gutters are 16px on phones, 24px from md. */
const CHAT_COLUMN = "mx-auto w-full max-w-3xl px-4 md:px-6";

type Session = ReturnType<typeof useChatSession>;
type Evidence = ReturnType<typeof useEvidence>;

function Thread({ session, evidence }: { session: Session; evidence: Evidence }) {
  const t = useTranslations("chat");
  const { messages, busy, status } = session;
  const lastIndex = messages.length - 1;
  const awaitingFirstChunk = status === "submitted" && messages.at(-1)?.role === "user";
  return (
    <div className={cn(CHAT_COLUMN, "pt-8")}>
      {messages.map((message, index) =>
        message.role === "user" ? (
          <UserMessage key={message.id} message={message} />
        ) : (
          <AssistantMessage
            key={message.id}
            message={message}
            streaming={busy && index === lastIndex}
            selectedCitation={evidence.messageId === message.id ? evidence.n : undefined}
            onCite={(n) => evidence.cite(message.id, n)}
            onDecide={session.decide}
            onInspect={() => evidence.inspect(message.id)}
            // The inline alert below already offers the retry after a failed stream.
            onRegenerate={index === lastIndex && status === "ready" ? session.retry : undefined}
          />
        ),
      )}
      {awaitingFirstChunk && <PendingReply />}
      {status === "error" && (
        <div className="mb-8 flex flex-wrap items-center gap-x-4 gap-y-2">
          <InlineAlert title={t("streamFailed")} />
          <Button variant="outline" onClick={session.retry}>
            <RotateCcw aria-hidden />
            {t("regenerate")}
          </Button>
        </div>
      )}
    </div>
  );
}

/**
 * The chat route: header, then either the hero (empty thread) or the thread, with the composer between them.
 * The composer keeps one place in the tree and only changes its layout, so its focus survives the first send.
 */
export function ChatSurface({
  chatId,
  initialMessages,
  title,
  folder,
  scope,
  modelChoice,
  conversationPath,
}: ChatSurfaceProps) {
  const session = useChatSession({ chatId, initialMessages, modelChoice, conversationPath });
  const evidence = useEvidence(false);
  const threadRef = useStickToBottom<HTMLDivElement>(session.messages);
  const empty = session.messages.length === 0;
  const lastAssistant = session.messages.findLast((m) => m.role === "assistant");
  const lastView = lastAssistant ? session.views.get(lastAssistant.id) : undefined;
  const panelView = evidence.messageId ? session.views.get(evidence.messageId) : lastView;

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <ChatHeader
          title={title}
          folder={folder}
          runId={lastView?.runId}
          onInspect={lastView ? () => evidence.inspect() : undefined}
        />
        {/*
          Empty: from md the hero, composer and source chips gather in the vertical middle; below md the hero
          takes the top third and the composer stays docked at the bottom so the keyboard never covers it.
        */}
        <div
          className={cn(
            "flex min-h-0 flex-1 flex-col",
            empty && "overflow-y-auto md:justify-center md:pb-12",
          )}
        >
          {empty ? (
            <HeroIntro className="order-1 pt-8 pb-6 max-md:flex-1 md:pb-8" />
          ) : (
            <div ref={threadRef} className="min-h-0 flex-1 overflow-y-auto">
              <Thread session={session} evidence={evidence} />
            </div>
          )}
          <div
            className={cn(
              CHAT_COLUMN,
              "shrink-0 pb-[max(1rem,env(safe-area-inset-bottom))]",
              empty && "order-3 md:order-2",
            )}
          >
            <Composer
              busy={session.busy}
              onSend={session.send}
              onStop={session.stop}
              models={modelChoice.models}
              modelId={session.modelId}
              onModelChange={session.setModelId}
              scope={scope}
              contextRatio={contextRatio(lastView, modelChoice, session.modelId)}
            />
          </div>
          {empty && <HeroSources className="order-2 pb-4 md:order-3 md:pb-0" />}
        </div>
      </div>
      <EvidenceDock evidence={evidence} view={panelView} />
    </div>
  );
}

function EvidenceDock({ evidence, view }: { evidence: Evidence; view: RunView | undefined }) {
  return (
    <EvidencePanel
      open={evidence.open}
      tab={evidence.tab}
      onTab={evidence.setTab}
      onClose={evidence.close}
      citations={view?.citations ?? []}
      selected={evidence.n}
      onSelect={evidence.selectInMessage}
      stations={view?.stations ?? []}
      usage={view?.usage}
      runId={view?.runId}
    />
  );
}
