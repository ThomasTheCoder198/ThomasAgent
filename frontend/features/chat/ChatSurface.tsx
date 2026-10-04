"use client";

import { RotateCcw } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";
import type { ModelChoice } from "@/features/registry/types";

import { AssistantMessage, UserMessage } from "./components/AssistantMessage";
import { ChatHeader } from "./components/ChatHeader";
import { Composer } from "./components/Composer";
import { NewChatIntro } from "./components/NewChatIntro";
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

const PENDING_REPLY: ChatMessage = { id: "pending", role: "assistant", parts: [] };
const noop = () => undefined;

/**
 * Thread and composer share one fluid column that grows with the window. On very wide screens it anchors
 * left (the run line's side) instead of floating as a centred strip; prose keeps its own 75ch measure.
 */
const CHAT_COLUMN = "mx-auto w-full max-w-240 2xl:mr-auto 2xl:ml-[clamp(2.5rem,6vw,8rem)] 2xl:max-w-288";

type Session = ReturnType<typeof useChatSession>;
type Evidence = ReturnType<typeof useEvidence>;

function Thread({ session, evidence }: { session: Session; evidence: Evidence }) {
  const t = useTranslations("chat");
  const { messages, busy, status } = session;
  const awaitingFirstChunk = status === "submitted" && messages.at(-1)?.role === "user";
  return (
    <div className={cn(CHAT_COLUMN, "px-4 md:px-8")}>
      {messages.length === 0 && <NewChatIntro />}
      {messages.map((message, index) =>
        message.role === "user" ? (
          <UserMessage key={message.id} message={message} />
        ) : (
          <AssistantMessage
            key={message.id}
            message={message}
            streaming={busy && index === messages.length - 1}
            selectedCitation={evidence.messageId === message.id ? evidence.n : undefined}
            onCite={(n) => evidence.cite(message.id, n)}
            onDecide={session.decide}
            onInspect={() => evidence.inspect(message.id)}
          />
        ),
      )}
      {awaitingFirstChunk && (
        <AssistantMessage message={PENDING_REPLY} streaming onCite={noop} onDecide={noop} onInspect={noop} />
      )}
      {status === "error" && (
        <div role="alert" className="text-app mb-6 flex items-center gap-3 text-sm">
          {t("streamFailed")}
          <Button variant="outline" onClick={session.retry}>
            <RotateCcw aria-hidden className="size-3.5" />
            {t("regenerate")}
          </Button>
        </div>
      )}
    </div>
  );
}

/** The chat route: header, thread with its run lines, composer, and the evidence sign on the right. */
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
        <div
          ref={threadRef}
          className="min-h-0 flex-1 overflow-y-auto [mask-image:linear-gradient(to_bottom,transparent_0,#000_28px)] pt-6.5"
        >
          <Thread session={session} evidence={evidence} />
        </div>
        <div className="from-ground/0 to-ground bg-gradient-to-b from-0% to-30% pt-3.5 pb-4.5">
          {/* Same column and same inner padding as the thread, so composer and messages share both edges. */}
          <div className={cn(CHAT_COLUMN, "px-4 md:px-8")}>
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
