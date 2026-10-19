"use client";

import { RotateCcw } from "lucide-react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { EmptyState } from "@/features/shell/EmptyState";
import type { ModelChoice } from "@/features/registry/types";
import { cn } from "@/lib/cn";
import type { Agent } from "@/lib/core/types";

import { AgentPicker } from "./components/AgentPicker";
import { AssistantMessage, PendingReply, UserMessage } from "./components/AssistantMessage";
import { ChatHeader } from "./components/ChatHeader";
import { HeroIntro, heroStep, heroStyle, HeroSources } from "./components/ChatHero";
import { Composer } from "./components/Composer";
import type { ChatMessage, ChatScope } from "./contract";
import { EvidencePanel } from "./evidence/EvidencePanel";
import type { RunView } from "./run-view";
import { scopeOfAgent } from "./scope";
import { contextRatio, useChatSession } from "./useChatSession";
import { useEvidence } from "./useEvidence";
import { useStickToBottom } from "./useStickToBottom";

export type ChatSurfaceProps = {
  orgId: string;
  chatId: string;
  initialMessages: ChatMessage[];
  title: string;
  /** The org's Agents a new chat can start on; empty for a stored conversation, which stays bound to its Agent (R12). */
  agents: Agent[];
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

/** A new chat needs an Agent to answer it; with none in the org there is nothing to send to. */
function NoAgent() {
  const t = useTranslations("chat");
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto">
      <EmptyState level="h2" line="model" title={t("noAgentTitle")} body={t("noAgentBody")}>
        <Button asChild>
          <Link href="/agents">{t("noAgentAction")}</Link>
        </Button>
      </EmptyState>
    </div>
  );
}

/** The latest run's view, and the one the evidence panel points at (the picked message, else the latest). */
function viewsOf(session: Session, evidence: Evidence) {
  const lastAssistant = session.messages.findLast((m) => m.role === "assistant");
  const lastView = lastAssistant ? session.views.get(lastAssistant.id) : undefined;
  const panelView = evidence.messageId ? session.views.get(evidence.messageId) : lastView;
  return { lastView, panelView };
}

type BodyProps = {
  session: Session;
  evidence: Evidence;
  lastView: RunView | undefined;
  agents: Agent[];
  agentId: string | undefined;
  onAgentChange: (id: string) => void;
  scope: ChatScope | null;
  modelChoice: ModelChoice;
};

/**
 * The hero (empty thread) or the thread, with the composer between them. The composer keeps one place in the
 * tree and only changes its layout, so its focus survives the first send.
 * Empty: from md the hero, picker, composer and source chips gather in the vertical middle; below md the hero and
 * picker take the top and the composer stays docked at the bottom so the keyboard never covers it.
 */
function ChatBody({
  session,
  evidence,
  lastView,
  agents,
  agentId,
  onAgentChange,
  scope,
  modelChoice,
}: BodyProps) {
  const threadRef = useStickToBottom<HTMLDivElement>(session.messages);
  const empty = session.messages.length === 0;
  return (
    <div
      className={cn("flex min-h-0 flex-1 flex-col", empty && "overflow-y-auto md:justify-center md:pb-12")}
    >
      {empty ? (
        <HeroIntro className="order-1 pt-8 pb-6 md:pb-8" />
      ) : (
        <div ref={threadRef} className="min-h-0 flex-1 overflow-y-auto [overflow-anchor:none]">
          <Thread session={session} evidence={evidence} />
        </div>
      )}
      {empty && (
        <div className={cn(CHAT_COLUMN, "order-1 pb-4 empty:hidden md:pb-6")}>
          <AgentPicker agents={agents} agentId={agentId} onChange={onAgentChange} />
        </div>
      )}
      <div
        // The composer keeps its place in the tree, so only the hero's entrance fades it in (3rd step).
        style={empty ? heroStyle(heroStep.composer) : undefined}
        className={cn(
          CHAT_COLUMN,
          "shrink-0 pb-[max(1rem,env(safe-area-inset-bottom))]",
          empty && "animate-rise order-3 md:order-2",
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
      {empty && <HeroSources className="order-2 pb-4 max-md:mb-auto md:order-3 md:pb-0" />}
    </div>
  );
}

/** The chat route: header, then the body, or the no-Agent state when a new chat has nobody to answer it. */
export function ChatSurface({
  orgId,
  chatId,
  initialMessages,
  title,
  agents,
  scope,
  modelChoice,
  conversationPath,
}: ChatSurfaceProps) {
  const [agentId, setAgentId] = useState(scope?.agent.id);
  const picked = agents.find((a) => a.id === agentId);
  // A new chat follows the picker; a stored conversation keeps the scope core returned (R12).
  const liveScope = picked ? scopeOfAgent(picked) : scope;
  const session = useChatSession({
    orgId,
    agentId,
    chatId,
    initialMessages,
    modelChoice,
    conversationPath,
  });
  const evidence = useEvidence(false);
  const { lastView, panelView } = viewsOf(session, evidence);
  const noAgent = session.messages.length === 0 && !liveScope;

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <ChatHeader
          title={title}
          agentName={liveScope?.agent.name}
          runId={lastView?.runId}
          onInspect={lastView ? () => evidence.inspect() : undefined}
        />
        {noAgent ? (
          <NoAgent />
        ) : (
          <ChatBody
            session={session}
            evidence={evidence}
            lastView={lastView}
            agents={agents}
            agentId={agentId}
            onAgentChange={setAgentId}
            scope={liveScope}
            modelChoice={modelChoice}
          />
        )}
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
