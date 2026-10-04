/**
 * The only place that knows the AI SDK message protocol. It turns a streamed `ChatMessage` into the
 * Metro view model the presentational components draw: stations on a line, answer text, citations, usage.
 */
import { isToolLine, type ToolLine } from "@/components/metro/lines";

import {
  METADATA_NS,
  type ChatMessage,
  type ChatPart,
  type CitationLocator,
  type RunUsage,
  type ToolMeta,
} from "./contract";

export type ToolState = "running" | "awaiting" | "done" | "error" | "denied";

export type ReasoningStation = {
  kind: "reasoning";
  id: string;
  text: string;
  live: boolean;
  durationMs?: number;
};
export type StatusStation = { kind: "status"; id: string; label: string };
export type ToolStation = {
  kind: "tool";
  id: string;
  name: string;
  /** Human title the runtime gives the tool (MCP annotations, App manifest); falls back to `name`. */
  title: string;
  line: ToolLine;
  source: string;
  state: ToolState;
  input: Record<string, unknown>;
  output?: unknown;
  errorText?: string;
  durationMs?: number;
  approvalId?: string;
  /** True when the owner explicitly allowed this call at its approval sign. */
  ownerApproved: boolean;
};
export type Station = ReasoningStation | StatusStation | ToolStation;

export type Segment =
  | { kind: "steps"; id: string; stations: Station[] }
  | { kind: "text"; id: string; text: string; streaming: boolean };

export type Citation = {
  n: number;
  sourceId: string;
  title: string;
  fileName: string;
  locator: CitationLocator | undefined;
};

export type RunView = {
  segments: Segment[];
  citations: Citation[];
  usage: RunUsage | undefined;
  runId: string | undefined;
  /** Every tool call in the run, for the summary row and the inspector. */
  stations: Station[];
};

type DynamicToolPart = Extract<ChatPart, { type: "dynamic-tool" }>;

const toolStates: Record<DynamicToolPart["state"], ToolState> = {
  "input-streaming": "running",
  "input-available": "running",
  "approval-requested": "awaiting",
  "approval-responded": "running",
  "output-available": "done",
  "output-error": "error",
  "output-denied": "denied",
};

function thomasMeta<T>(metadata: unknown): T | undefined {
  return (metadata as Record<string, T> | undefined)?.[METADATA_NS];
}

function toolMeta(part: DynamicToolPart): ToolMeta {
  // Metadata arrives over the wire; an unknown line falls back to Apps rather than inventing a colour.
  const meta = part.toolMetadata as { line?: unknown; source?: string; durationMs?: number } | undefined;
  const line = isToolLine(meta?.line) ? meta.line : "app";
  return { line, source: meta?.source ?? part.toolName, durationMs: meta?.durationMs };
}

function toToolStation(part: DynamicToolPart): ToolStation {
  const meta = toolMeta(part);
  return {
    kind: "tool",
    id: part.toolCallId,
    name: part.toolName,
    title: part.title ?? part.toolName,
    line: meta.line,
    source: meta.source,
    state: toolStates[part.state],
    input: (part.input ?? {}) as Record<string, unknown>,
    output: part.state === "output-available" ? part.output : undefined,
    errorText: part.state === "output-error" ? part.errorText : undefined,
    ownerApproved: part.approval?.approved === true,
    durationMs: meta.durationMs,
    approvalId: part.state === "approval-requested" ? part.approval.id : undefined,
  };
}

function toStation(part: ChatPart, index: number): Station | undefined {
  if (part.type === "reasoning") {
    const duration = thomasMeta<{ durationMs: number }>(part.providerMetadata)?.durationMs;
    return {
      kind: "reasoning",
      id: part.id ?? `r${index}`,
      text: part.text,
      live: part.state === "streaming",
      durationMs: duration,
    };
  }
  if (part.type === "dynamic-tool") return toToolStation(part);
  if (part.type === "data-status")
    return { kind: "status", id: part.id ?? `s${index}`, label: part.data.label };
  return undefined;
}

function toCitation(part: Extract<ChatPart, { type: "source-document" }>, n: number): Citation {
  return {
    n,
    sourceId: part.sourceId,
    title: part.title,
    fileName: part.filename ?? part.title,
    locator: thomasMeta<CitationLocator>(part.providerMetadata),
  };
}

function pushStation(segments: Segment[], station: Station) {
  const last = segments.at(-1);
  if (last?.kind === "steps") last.stations.push(station);
  else segments.push({ kind: "steps", id: `steps-${station.id}`, stations: [station] });
}

export function toRunView(message: ChatMessage): RunView {
  const view: RunView = {
    segments: [],
    citations: [],
    usage: undefined,
    runId: message.metadata?.runId,
    stations: [],
  };
  message.parts.forEach((part, index) => {
    if (part.type === "text") {
      if (part.text.length > 0)
        view.segments.push({
          kind: "text",
          id: `t${index}`,
          text: part.text,
          streaming: part.state === "streaming",
        });
      return;
    }
    if (part.type === "source-document") {
      view.citations.push(toCitation(part, view.citations.length + 1));
      return;
    }
    if (part.type === "data-usage") {
      view.usage = part.data;
      return;
    }
    const station = toStation(part, index);
    if (station) {
      pushStation(view.segments, station);
      view.stations.push(station);
    }
  });
  return view;
}

/** Stations that actually ran: a tool still waiting for approval (or denied) is not part of the route yet. */
export function travelled(stations: readonly Station[]): Station[] {
  return stations.filter((s) => s.kind !== "tool" || s.state === "done" || s.state === "error");
}

/** A run is paused, not finished, while any tool waits for the owner's answer. */
export function isAwaitingApproval(stations: readonly Station[]): boolean {
  return stations.some((s) => s.kind === "tool" && s.state === "awaiting");
}

export function toolCount(stations: readonly Station[]): number {
  return stations.filter((s) => s.kind === "tool").length;
}

export function totalDurationMs(stations: readonly Station[]): number {
  return stations.reduce((sum, s) => sum + (s.kind === "status" ? 0 : (s.durationMs ?? 0)), 0);
}

/** The line colours a run touched, in order, for the collapsed mini line. */
export function runLines(stations: readonly Station[]): Array<ToolLine | "model"> {
  return stations.map((s) => (s.kind === "tool" ? s.line : "model"));
}
