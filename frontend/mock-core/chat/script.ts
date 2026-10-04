import type { RunUsage } from "../../features/chat/contract.ts";
import { pacing } from "../config.ts";
import { citations, KB, refundScript } from "../fixtures/refund-policy.ts";
import {
  finishTool,
  runTool,
  startTool,
  streamReasoning,
  streamText,
  type RunClock,
  type Writer,
} from "./writer.ts";

/**
 * The scripted agent run behind every mock chat. Phase one thinks, searches the KB, cites and answers,
 * then stops at a risky Linear call waiting for approval; phase two finishes after the owner decides.
 */
const KB_META = { line: "kb", source: KB.name } as const;
const LINEAR_META = { line: "mcp", source: refundScript.linear.source } as const;
const USAGE = { inputTokens: 2_914, outputTokens: 568, costUsd: 0.0061, ttftMs: 820 } as const;
const RUN_DURATION_MS = 4_100;
const SEARCH_CHUNKS = 8;
const READ_BLOCKS = 4;

function linearCall(runId: string) {
  const { title, input } = refundScript.linear;
  return { id: `${runId}-linear`, name: "linear_create_issue", title, meta: LINEAR_META, input };
}

export async function runPhaseOne(writer: Writer, clock: RunClock, runId: string): Promise<boolean> {
  writer.write({ type: "start", messageMetadata: { runId } });
  if (!(await clock.wait(pacing.firstChunk))) return false;
  if (
    !(await streamReasoning(writer, clock, `${runId}-r1`, refundScript.reasoningPlan, pacing.reasoningWord))
  )
    return false;
  const search = {
    id: `${runId}-kb1`,
    name: "kb_search",
    meta: KB_META,
    input: { query: refundScript.kbQuery },
  };
  const found = { chunks: SEARCH_CHUNKS, documents: [...new Set(citations.map((c) => c.fileName))] };
  if (!(await runTool(writer, clock, search, found, pacing.kbSearch))) return false;
  const read = {
    id: `${runId}-kb2`,
    name: "kb_read_section",
    meta: KB_META,
    input: refundScript.readSection,
  };
  if (!(await runTool(writer, clock, read, { blocks: READ_BLOCKS }, pacing.kbRead))) return false;
  if (!(await clock.wait(pacing.betweenStations))) return false;
  if (
    !(await streamReasoning(
      writer,
      clock,
      `${runId}-r2`,
      refundScript.reasoningCompose,
      pacing.reasoningWord,
    ))
  )
    return false;
  for (const c of citations) {
    writer.write({
      type: "source-document",
      sourceId: c.sourceId,
      mediaType: "application/pdf",
      title: c.title,
      filename: c.fileName,
      providerMetadata: { thomas: c.locator },
    });
  }
  if (!(await streamText(writer, clock, `${runId}-t1`, refundScript.answer, pacing.textWord))) return false;
  const linear = linearCall(runId);
  startTool(writer, linear);
  writer.write({ type: "tool-approval-request", approvalId: `${runId}-approval`, toolCallId: linear.id });
  writer.write({ type: "finish" });
  return true;
}

export async function runPhaseTwo(writer: Writer, clock: RunClock, runId: string, approved: boolean) {
  writer.write({ type: "start", messageMetadata: { runId } });
  const linear = linearCall(runId);
  if (approved) {
    if (!(await clock.wait(pacing.toolCall))) return false;
    finishTool(writer, linear, refundScript.linear.output, pacing.toolCall);
  } else {
    writer.write({ type: "tool-output-denied", toolCallId: linear.id });
  }
  const closing = approved ? refundScript.afterApproval : refundScript.afterDenial;
  if (!(await streamText(writer, clock, `${runId}-t2`, closing, pacing.textWord))) return false;
  const usage: RunUsage = { runId, ...USAGE, durationMs: RUN_DURATION_MS };
  writer.write({ type: "data-usage", id: `${runId}-usage`, data: usage });
  writer.write({ type: "finish" });
  return true;
}
