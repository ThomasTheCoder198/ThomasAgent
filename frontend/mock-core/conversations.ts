import { createUIMessageStream, readUIMessageStream, type UIMessageStreamWriter } from "ai";

import type { ChatMessage, ConversationDetail, ConversationSummary } from "../features/chat/contract.ts";
import { scopeOfAgent } from "../features/chat/scope.ts";
import type { RunSummary } from "../lib/core/types.ts";
import { listAgentsOf } from "./agents.ts";
import { runPhaseOne, runPhaseTwo } from "./chat/script.ts";
import { RunClock } from "./chat/writer.ts";
import { refundScript } from "./fixtures/refund-policy.ts";

/** In-memory conversation store, per org; it resets whenever the fake core restarts. */
const HOUR_MS = 3_600_000;
const SEEDED_RUN = "R-0142";

type Stored = ConversationDetail & { lines: string[]; updatedAt: string };

const stores = new Map<string, Map<string, Stored>>();

function hoursAgo(hours: number) {
  return new Date(Date.now() - hours * HOUR_MS).toISOString();
}

function userMessage(id: string, text: string): ChatMessage {
  return { id, role: "user", parts: [{ type: "text", text }] };
}

async function collect(
  run: (writer: UIMessageStreamWriter<ChatMessage>) => Promise<unknown>,
  start?: ChatMessage,
): Promise<ChatMessage> {
  const stream = createUIMessageStream<ChatMessage>({
    execute: ({ writer }) => run(writer).then(() => undefined),
  });
  let last = start;
  for await (const message of readUIMessageStream<ChatMessage>({ message: start, stream })) last = message;
  if (!last) throw new Error("scripted run produced no message");
  return last;
}

/** Builds the finished refund conversation by replaying the same script the live mock streams, approved. */
async function seededRefundMessages(): Promise<ChatMessage[]> {
  const instant = 0;
  const clock = new RunClock(instant);
  const asked = await collect((w) => runPhaseOne(w, clock, SEEDED_RUN));
  const approved: ChatMessage = {
    ...asked,
    parts: asked.parts.map((part) =>
      part.type === "dynamic-tool" && part.state === "approval-requested"
        ? { ...part, state: "approval-responded", approval: { ...part.approval, approved: true } }
        : part,
    ),
  };
  const done = await collect((w) => runPhaseTwo(w, clock, SEEDED_RUN, true), approved);
  return [userMessage(`${SEEDED_RUN}-u`, refundScript.question), done];
}

/** Replayed once at start-up; every org's seeded refund conversation shares these messages. */
const refundMessages = await seededRefundMessages();

const seedTitles: Array<{ id: string; title: string; lines: string[]; hours: number }> = [
  { id: "c-hoan-tien-2026", title: "Chính sách hoàn tiền 2026", lines: ["kb", "mcp"], hours: 1 },
  { id: "c-ncc-a-b", title: "So sánh hợp đồng NCC A và B", lines: ["kb"], hours: 5 },
  { id: "c-email-tuan-39", title: "Tổng hợp email khách tuần 39", lines: ["cmp"], hours: 26 },
  { id: "c-doanh-thu-q3", title: "Doanh thu Q3 theo kênh — Sheet", lines: ["kb", "cmp"], hours: 30 },
  { id: "c-pr-128", title: "Review PR #128 ingest worker", lines: ["mcp"], hours: 52 },
  { id: "c-hop-phap-che", title: "Lịch họp với đội pháp chế", lines: ["cmp"], hours: 75 },
  { id: "c-rrf-rerank", title: "Giải thích RRF vs rerank", lines: [], hours: 98 },
];

/** Seeded on first read per org, bound to the org's first Agent; with no Agent there is no seed. */
function storeOf(orgId: string): Map<string, Stored> {
  const existing = stores.get(orgId);
  if (existing) return existing;
  const store = new Map<string, Stored>();
  const agent = listAgentsOf(orgId)[0];
  if (agent) {
    for (const { id, title, lines, hours } of seedTitles) {
      const isRefund = id === "c-hoan-tien-2026";
      store.set(id, {
        id,
        title,
        agentId: agent.id,
        lines,
        updatedAt: hoursAgo(hours),
        scope: scopeOfAgent(agent),
        messages: isRefund ? refundMessages : [userMessage(`${id}-u`, title)],
        lastRun: isRefund ? { id: SEEDED_RUN, status: "completed", errorCode: "", requestId: "" } : null,
      });
    }
  }
  stores.set(orgId, store);
  return store;
}

/** `POST /__mock/reset`: forget every org's conversations; they reseed on the next read. */
export function resetConversations() {
  stores.clear();
}

export function listConversations(orgId: string): ConversationSummary[] {
  return [...storeOf(orgId).values()]
    .map(({ id, title, agentId, lines, updatedAt }) => ({ id, title, agentId, lines, updatedAt }))
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}

export function getConversation(orgId: string, id: string): ConversationDetail | undefined {
  const found = storeOf(orgId).get(id);
  if (!found) return undefined;
  const { id: foundId, title, agentId, scope, messages, lastRun } = found;
  return { id: foundId, title, agentId, scope, messages, lastRun };
}

const TITLE_LIMIT = 60;

function titleOf(messages: ChatMessage[], fallback: string): string {
  const firstText = messages.flatMap((m) => m.parts).find((p) => p.type === "text");
  return firstText?.type === "text" ? firstText.text.slice(0, TITLE_LIMIT) : fallback;
}

export function saveConversation(
  orgId: string,
  id: string,
  agentId: string,
  messages: ChatMessage[],
  lastRun: RunSummary | null = null,
) {
  const store = storeOf(orgId);
  const existing = store.get(id);
  const agent = listAgentsOf(orgId).find((a) => a.id === (existing?.agentId ?? agentId));
  if (!agent) return;
  store.set(id, {
    id,
    title: existing?.title ?? titleOf(messages, id),
    agentId: agent.id,
    lines: ["kb"],
    updatedAt: new Date().toISOString(),
    scope: existing?.scope ?? scopeOfAgent(agent),
    messages,
    lastRun: lastRun ?? existing?.lastRun ?? null,
  });
}
