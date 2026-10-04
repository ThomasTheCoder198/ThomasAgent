"use client";

import { useId, useState } from "react";

import { cn } from "@/lib/cn";

import type { ChatMessage } from "../contract";
import { isAwaitingApproval, toRunView, travelled, type Segment, type Station } from "../run-view";
import { Answer } from "./Answer";
import type { ApprovalDecision } from "./ApprovalSign";
import { MessageFoot } from "./MessageFoot";
import { RunLine } from "./RunLine";
import { SummaryRow } from "./SummaryRow";

type Props = {
  message: ChatMessage;
  streaming: boolean;
  selectedCitation?: number;
  onCite: (n: number) => void;
  onDecide: (decision: ApprovalDecision) => void;
  onInspect: () => void;
};

const PENDING_STATION: Station = { kind: "reasoning", id: "pending", text: "", live: true };

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
}: Props) {
  const view = toRunView(message);
  const [expanded, setExpanded] = useState(false);
  const stepsId = useId();
  const segments: Segment[] =
    view.segments.length === 0 && streaming
      ? [{ kind: "steps", id: "pending", stations: [PENDING_STATION] }]
      : view.segments;
  const liveId = liveStationId(segments, streaming);
  const hasText = segments.some((s) => s.kind === "text");
  // Once the answer exists the opening stations fold into the summary row; later tool stations stay in place.
  const foldFirst = hasText && !streaming;
  const paused = isAwaitingApproval(view.stations);
  const pendingLine = view.stations.find((s) => s.kind === "tool" && s.state === "awaiting");
  const citationNumbers = view.citations.map((c) => c.n);

  return (
    <article className="mb-7" aria-busy={streaming}>
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
      {!streaming && hasText && !paused && (
        <MessageFoot text={plainText(segments)} usage={view.usage} onInspect={onInspect} />
      )}
    </article>
  );
}

export function UserMessage({ message }: { message: ChatMessage }) {
  const text = message.parts.flatMap((p) => (p.type === "text" ? [p.text] : [])).join("\n");
  return (
    <div className="animate-rise mb-5.5 flex justify-end">
      <p className="border-rule bg-paper max-w-[78%] rounded-[14px_14px_4px_14px] border px-3.75 py-2.75 whitespace-pre-wrap">
        {text}
      </p>
    </div>
  );
}
