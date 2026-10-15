"use client";

import { useTranslations } from "next-intl";
import { useId, useState } from "react";

import { cn } from "@/lib/cn";

import type { ChatMessage } from "../contract";
import { isAwaitingApproval, toRunView, travelled, type Segment } from "../run-view";
import { Answer } from "./Answer";
import type { ApprovalDecision } from "./ApprovalSign";
import { MessageFoot } from "./MessageFoot";
import { OrbNote } from "./OrbNote";
import { RunLine } from "./RunLine";
import { SummaryRow } from "./SummaryRow";

type Props = {
  message: ChatMessage;
  streaming: boolean;
  selectedCitation?: number;
  onCite: (n: number) => void;
  onDecide: (decision: ApprovalDecision) => void;
  onInspect: () => void;
  /** Present only on the newest finished answer. */
  onRegenerate?: () => void;
};

/** Before the first part arrives, whether the request is queued or the stream is open but silent. */
export function PendingReply() {
  const t = useTranslations("chat");
  return (
    <OrbNote state="solving" size={32} live className="mb-8">
      {t("thinkingLive")}
    </OrbNote>
  );
}

/** While streaming, the newest station is live unless the answer text has taken over. */
function liveStationId(segments: Segment[], streaming: boolean): string | undefined {
  const last = segments.at(-1);
  if (!streaming || last?.kind !== "steps") return undefined;
  return last.stations.at(-1)?.id;
}

function plainText(segments: Segment[]) {
  return segments.flatMap((s) => (s.kind === "text" ? [s.text] : [])).join("\n\n");
}

export function AssistantMessage({
  message,
  streaming,
  selectedCitation,
  onCite,
  onDecide,
  onInspect,
  onRegenerate,
}: Props) {
  const t = useTranslations("chat");
  const view = toRunView(message);
  const [expanded, setExpanded] = useState(false);
  const stepsId = useId();
  const segments = view.segments;
  const liveId = liveStationId(segments, streaming);
  const hasText = segments.some((s) => s.kind === "text");
  // Once the answer exists the opening stations fold into the summary row; later tool stations stay in place.
  const foldFirst = hasText && !streaming;
  const paused = isAwaitingApproval(view.stations);
  const pendingLine = view.stations.find((s) => s.kind === "tool" && s.state === "awaiting");
  const citationNumbers = view.citations.map((c) => c.n);
  if (segments.length === 0 && streaming) return <PendingReply />;

  return (
    <article className="mb-8" aria-busy={streaming}>
      {segments.map((segment, index) => {
        if (segment.kind === "text") {
          return (
            <Answer
              key={segment.id}
              text={segment.text}
              streaming={segment.streaming && streaming}
              citationNumbers={citationNumbers}
              selected={selectedCitation}
              onCite={onCite}
            />
          );
        }
        if (index === 0 && foldFirst) {
          return (
            <div key={segment.id}>
              <SummaryRow
                stations={travelled(view.stations)}
                pendingLine={pendingLine?.kind === "tool" ? pendingLine.line : undefined}
                expanded={expanded}
                onToggle={() => setExpanded((v) => !v)}
                controls={stepsId}
              />
              {/* Unfolds by animating grid rows 0fr → 1fr; `inert` keeps folded stations out of tab order. */}
              <div
                id={stepsId}
                inert={!expanded}
                className={cn(
                  "ease-out-expo grid transition-[grid-template-rows,opacity] duration-300",
                  expanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
                )}
              >
                <div className="min-h-0 overflow-hidden">
                  <RunLine stations={segment.stations} onDecide={onDecide} />
                </div>
              </div>
            </div>
          );
        }
        return (
          <RunLine
            key={segment.id}
            stations={segment.stations}
            liveId={liveId}
            onDecide={onDecide}
            deciding={streaming}
          />
        );
      })}
      {streaming && segments.at(-1)?.kind === "text" && (
        <OrbNote state="composing" size={20} className="mt-2 text-[0.8125rem]">
          {t("writingLive")}
        </OrbNote>
      )}
      {!streaming && hasText && !paused && (
        <MessageFoot
          text={plainText(segments)}
          usage={view.usage}
          onInspect={onInspect}
          onRegenerate={onRegenerate}
        />
      )}
    </article>
  );
}

export function UserMessage({ message }: { message: ChatMessage }) {
  const text = message.parts.flatMap((p) => (p.type === "text" ? [p.text] : [])).join("\n");
  return (
    <div className="animate-rise mb-8 flex justify-end">
      <p className="border-border bg-card max-w-[80%] rounded-lg rounded-br-sm border px-4 py-3 text-[0.9375rem] break-words whitespace-pre-wrap">
        {text}
      </p>
    </div>
  );
}
