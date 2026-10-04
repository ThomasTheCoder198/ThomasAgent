import type { UIMessageStreamWriter } from "ai";

import {
  METADATA_NS,
  type ChatMessage,
  type ReasoningMeta,
  type ToolMeta,
} from "../../features/chat/contract.ts";
import { mockConfig } from "../config.ts";

export type Writer = UIMessageStreamWriter<ChatMessage>;

/** Stops a scripted run as soon as the browser hangs up (the Stop button aborts the fetch). */
export class RunClock {
  private aborted = false;
  private readonly speed: number;
  constructor(speed: number = mockConfig.speed) {
    this.speed = speed;
  }
  abort() {
    this.aborted = true;
  }
  get stopped() {
    return this.aborted;
  }
  async wait(ms: number) {
    const scaled = ms * this.speed;
    if (scaled > 0) await new Promise((resolve) => setTimeout(resolve, scaled));
    return !this.aborted;
  }
}

function words(text: string): string[] {
  return text.match(/\S+\s*|\s+/g) ?? [];
}

export async function streamReasoning(
  writer: Writer,
  clock: RunClock,
  id: string,
  text: string,
  perWord: number,
) {
  const tokens = words(text);
  writer.write({ type: "reasoning-start", id });
  for (const word of tokens) {
    if (!(await clock.wait(perWord))) return false;
    writer.write({ type: "reasoning-delta", id, delta: word });
  }
  // Scripted, not measured, so seeded conversations built at speed 0 still show believable timings.
  const meta: ReasoningMeta = { durationMs: tokens.length * perWord };
  writer.write({ type: "reasoning-end", id, providerMetadata: { [METADATA_NS]: meta } });
  return true;
}

export async function streamText(writer: Writer, clock: RunClock, id: string, text: string, perWord: number) {
  writer.write({ type: "text-start", id });
  for (const word of words(text)) {
    if (!(await clock.wait(perWord))) return false;
    writer.write({ type: "text-delta", id, delta: word });
  }
  writer.write({ type: "text-end", id });
  return true;
}

type ToolCall = { id: string; name: string; title?: string; meta: ToolMeta; input: Record<string, unknown> };

export function startTool(writer: Writer, call: ToolCall) {
  const common = {
    toolCallId: call.id,
    toolName: call.name,
    title: call.title,
    dynamic: true,
    toolMetadata: call.meta,
  };
  writer.write({ type: "tool-input-start", ...common });
  writer.write({ type: "tool-input-available", ...common, input: call.input });
}

export function finishTool(writer: Writer, call: ToolCall, output: unknown, durationMs: number) {
  writer.write({
    type: "tool-output-available",
    toolCallId: call.id,
    output,
    dynamic: true,
    toolMetadata: { ...call.meta, durationMs },
  });
}

export async function runTool(writer: Writer, clock: RunClock, call: ToolCall, output: unknown, ms: number) {
  startTool(writer, call);
  if (!(await clock.wait(ms))) return false;
  finishTool(writer, call, output, ms);
  return true;
}
