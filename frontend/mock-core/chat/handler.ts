import type { IncomingMessage, ServerResponse } from "node:http";

import { createUIMessageStream, pipeUIMessageStreamToResponse } from "ai";

import type { ChatMessage, ChatRequestBody } from "../../features/chat/contract.ts";
import type { RunSummary } from "../../lib/core/types.ts";
import { ErrorCode } from "../../lib/errors/codes.gen.ts";
import { listAgentsOf } from "../agents.ts";
import { currentUserId } from "../auth.ts";
import { getConversation, saveConversation } from "../conversations.ts";
import { readJson, sendData, sendError } from "../http.ts";
import { runPhaseOne, runPhaseTwo } from "./script.ts";
import { RunClock } from "./writer.ts";

const SSE_HEADERS = { "Cache-Control": "no-cache, no-transform" };
const HTTP_ACCEPTED = 202;
const RUN_BASE = 143;
const RUN_ID_DIGITS = 4;
let runCounter = RUN_BASE;

/** Runs the fake core has seen, by id: Stop must check the same org and user that began each run. */
const runs = new Map<
  string,
  { clock: RunClock; status: RunSummary["status"]; orgId: string; userId: string }
>();

/** `POST /__mock/reset`: forget every run. */
export function resetRuns() {
  runs.clear();
}

function nextRunId() {
  runCounter += 1;
  return `R-${String(runCounter).padStart(RUN_ID_DIGITS, "0")}`;
}

function runOwner(req: IncomingMessage) {
  return currentUserId(req) ?? "";
}

/** The decision the owner made on the pending approval, if the request carries one. */
function approvalDecision(message: ChatMessage): boolean | undefined {
  for (const part of message.parts) {
    if (part.type === "dynamic-tool" && part.state === "approval-responded") return part.approval.approved;
  }
  return undefined;
}

/** History lives server-side (spec §7.1): the browser only sends its newest message. */
function history(orgId: string, chatId: string, incoming: ChatMessage): ChatMessage[] {
  const previous = getConversation(orgId, chatId)?.messages ?? [];
  const index = previous.findIndex((m) => m.id === incoming.id);
  return index === -1 ? [...previous, incoming] : [...previous.slice(0, index), incoming];
}

export async function chat(req: IncomingMessage, res: ServerResponse, orgId: string) {
  const body = await readJson<ChatRequestBody>(req);
  if (!body?.message) {
    sendError(req, res, ErrorCode.VALIDATION_FAILED);
    return;
  }
  // A new conversation takes the Agent the request names (R12); a stored one keeps its own.
  const stored = getConversation(orgId, body.id);
  const agentId = stored?.agentId ?? body.agentId;
  if (!agentId || !listAgentsOf(orgId).some((a) => a.id === agentId)) {
    sendError(req, res, ErrorCode.AGENT_NOT_FOUND);
    return;
  }
  const clock = new RunClock();
  res.on("close", () => clock.abort());
  const decision = body.message.role === "assistant" ? approvalDecision(body.message) : undefined;
  const runId = body.message.metadata?.runId ?? nextRunId();
  const requestId = req.headers["x-request-id"];
  const run = { clock, status: "running" as RunSummary["status"], orgId, userId: runOwner(req) };
  runs.set(runId, run);
  const originalMessages = history(orgId, body.id, body.message);

  const stream = createUIMessageStream<ChatMessage>({
    originalMessages,
    execute: async ({ writer }) => {
      if (decision === undefined) await runPhaseOne(writer, clock, runId);
      else await runPhaseTwo(writer, clock, runId, decision);
    },
    onEnd: ({ messages }) => {
      const status = clock.stopped ? "cancelled" : "completed";
      run.status = status;
      saveConversation(orgId, body.id, agentId, messages, {
        id: runId,
        status,
        errorCode: "",
        requestId: typeof requestId === "string" ? requestId : "",
      });
    },
  });
  // `no-transform` stops intermediaries (including Next's gzip on rewrites) from buffering the stream.
  // Real core's SSE handler needs the same header; see the S8 note in the frontend README.
  await pipeUIMessageStreamToResponse({ response: res, stream, headers: SSE_HEADERS });
}

/** `POST /api/v1/orgs/{orgId}/runs/{runId}/stop` (ruling PW-2c1): idempotent `202 {runId, status}`. */
export function stopRun(req: IncomingMessage, res: ServerResponse, orgId: string, runId: string) {
  const run = runs.get(runId);
  if (!run || run.orgId !== orgId || run.userId !== currentUserId(req)) {
    sendError(req, res, ErrorCode.RUN_NOT_ACTIVE);
    return;
  }
  if (run.status === "running") {
    run.clock.abort();
    run.status = "cancelled";
  }
  sendData(res, { runId, status: run.status }, HTTP_ACCEPTED);
}
