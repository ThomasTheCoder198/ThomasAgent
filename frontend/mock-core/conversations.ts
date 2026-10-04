import { createUIMessageStream, readUIMessageStream, type UIMessageStreamWriter } from "ai";

import type {
  ChatMessage,
  ChatScope,
  ConversationDetail,
  ConversationSummary,
} from "../features/chat/contract.ts";
import { runPhaseOne, runPhaseTwo } from "./chat/script.ts";
import { RunClock } from "./chat/writer.ts";
import { KB, refundScript } from "./fixtures/refund-policy.ts";

/** In-memory conversation store; it resets whenever the fake core restarts. */
export const defaultScope: ChatScope = {
  agent: { id: "agent-phap-che", name: "Trợ lý pháp chế" },
  kb: { id: KB.id, name: KB.name },
  toolSources: [
    { line: "mcp", name: "Linear" },
    { line: "cmp", name: "Gmail" },
    { line: "app", name: "Hoá đơn" },
  ],
};

const HOUR_MS = 3_600_000;
const SEEDED_RUN = "R-0142";

const store = new Map<string, ConversationDetail>();

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

const seedTitles: Array<Omit<ConversationSummary, "updatedAt"> & { hours: number }> = [
  { id: "c-hoan-tien-2026", title: "Chính sách hoàn tiền 2026", lines: ["kb", "mcp"], hours: 1 },
  { id: "c-ncc-a-b", title: "So sánh hợp đồng NCC A và B", lines: ["kb"], hours: 5 },
  { id: "c-email-tuan-39", title: "Tổng hợp email khách tuần 39", lines: ["cmp"], hours: 26 },
  { id: "c-doanh-thu-q3", title: "Doanh thu Q3 theo kênh — Sheet", lines: ["kb", "cmp"], hours: 30 },
  { id: "c-pr-128", title: "Review PR #128 ingest worker", lines: ["mcp"], hours: 52 },
  { id: "c-hop-phap-che", title: "Lịch họp với đội pháp chế", lines: ["cmp"], hours: 75 },
  { id: "c-rrf-rerank", title: "Giải thích RRF vs rerank", lines: [], hours: 98 },
];

export async function seedConversations() {
  const refund = await seededRefundMessages();
  for (const { hours, ...summary } of seedTitles) {
    const isRefund = summary.id === "c-hoan-tien-2026";
    store.set(summary.id, {
      ...summary,
      updatedAt: hoursAgo(hours),
      folder: isRefund ? "Pháp chế" : undefined,
      scope: defaultScope,
      messages: isRefund ? refund : [userMessage(`${summary.id}-u`, summary.title)],
    });
  }
}

export function listConversations(): ConversationSummary[] {
  return [...store.values()]
    .map(({ id, title, lines, updatedAt }) => ({ id, title, lines, updatedAt }))
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}

export function getConversation(id: string): ConversationDetail | undefined {
  return store.get(id);
}

const TITLE_LIMIT = 60;

export function saveConversation(id: string, messages: ChatMessage[]) {
  const existing = store.get(id);
  const firstText = messages.flatMap((m) => m.parts).find((p) => p.type === "text");
  const title = existing?.title ?? (firstText?.type === "text" ? firstText.text.slice(0, TITLE_LIMIT) : id);
  store.set(id, {
    id,
    title,
    lines: ["kb", "mcp"],
    updatedAt: new Date().toISOString(),
    scope: existing?.scope ?? defaultScope,
    folder: existing?.folder,
    messages,
  });
}
