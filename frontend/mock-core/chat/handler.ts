import type { IncomingMessage, ServerResponse } from "node:http";

import { createUIMessageStream, pipeUIMessageStreamToResponse } from "ai";

import type { ChatMessage, ChatRequestBody } from "../../features/chat/contract.ts";
import { ErrorCode } from "../../lib/errors/codes.gen.ts";
import { getConversation, saveConversation } from "../conversations.ts";
import { readJson, sendError } from "../http.ts";
import { runPhaseOne, runPhaseTwo } from "./script.ts";
import { RunClock } from "./writer.ts";

const SSE_HEADERS = { "Cache-Control": "no-cache, no-transform" };
const RUN_BASE = 143;
const RUN_ID_DIGITS = 4;
let runCounter = RUN_BASE;

function nextRunId() {
  runCounter += 1;
  return `R-${String(runCounter).padStart(RUN_ID_DIGITS, "0")}`;
}

/** The decision the owner made on the pending approval, if the request carries one. */
function approvalDecision(message: ChatMessage): boolean | undefined {
  for (const part of message.parts) {
    if (part.type === "dynamic-tool" && part.state === "approval-responded") return part.approval.approved;
  }
  return undefined;
}

/** History lives server-side (spec §7.1): the browser only sends its newest message. */
function history(chatId: string, incoming: ChatMessage): ChatMessage[] {
  const previous = getConversation(chatId)?.messages ?? [];
  const index = previous.findIndex((m) => m.id === incoming.id);
  return index === -1 ? [...previous, incoming] : [...previous.slice(0, index), incoming];
}

export async function chat(req: IncomingMessage, res: ServerResponse) {
  const body = await readJson<ChatRequestBody>(req);
  if (!body?.message) {
    sendError(req, res, ErrorCode.VALIDATION_FAILED);
    return;
  }
  const clock = new RunClock();
  res.on("close", () => clock.abort());
  const decision = body.message.role === "assistant" ? approvalDecision(body.message) : undefined;
  const runId = body.message.metadata?.runId ?? nextRunId();
  const originalMessages = history(body.id, body.message);

  const stream = createUIMessageStream<ChatMessage>({
    originalMessages,
    execute: async ({ writer }) => {
      if (decision === undefined) await runPhaseOne(writer, clock, runId);
      else await runPhaseTwo(writer, clock, runId, decision);
    },
    onEnd: ({ messages }) => saveConversation(body.id, messages),
  });
  // `no-transform` stops intermediaries (including Next's gzip on rewrites) from buffering the stream.
  // Real core's SSE handler needs the same header; see the S8 note in the frontend README.
  await pipeUIMessageStreamToResponse({ response: res, stream, headers: SSE_HEADERS });
}
