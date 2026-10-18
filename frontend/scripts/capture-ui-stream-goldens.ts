/**
 * Writes the UI Message Stream goldens that core's Go encoder is tested against (plan M2b, task 2b-3).
 * The chunks mirror what core emits; the AI SDK turns them into SSE bytes and into the assistant message the browser
 * would build. Run with `node scripts/capture-ui-stream-goldens.ts` from frontend/.
 */
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { createUIMessageStream, createUIMessageStreamResponse, type UIMessage } from "ai";

const OUT = join(import.meta.dirname, "../../backend-core/internal/chat/uistream/testdata");
const RUN_ID = "11111111-2222-4333-8444-555555555555";
const MESSAGE_ID = "msg-golden";
const KB = { line: "kb", source: "Knowledge Base" } as const;
/** A kb_search tool part's output is ragx's data after core's run-wide renumbering (index amendment 7). */
const SEARCH_OUTPUT = {
  results: [
    {
      source: 1,
      chunkId: "c-1",
      fileId: "2b1e7c3a-0000-4000-8000-000000000002",
      fileName: "policy.pdf",
      nodeId: "9b2f0c1e-0000-4000-8000-000000000001",
      section: ["Chính sách", "Hoàn tiền"],
      pageStart: 3,
      pageEnd: 3,
      score: 0.91,
      blocks: [
        {
          label: "[1.1]",
          blockId: "b-1",
          text: "Khách được hoàn tiền trong 7 ngày.",
          locator: { kind: "pdf_bbox", page: 3, bbox: [10, 20, 300, 80] },
        },
      ],
    },
  ],
  insufficientEvidence: false,
};

type Writer = Parameters<Parameters<typeof createUIMessageStream<UIMessage>>[0]["execute"]>[0]["writer"];
type Chunk = Parameters<Writer["write"]>[0];

const kbRun: Chunk[] = [
  { type: "start", messageId: MESSAGE_ID, messageMetadata: { runId: RUN_ID } },
  { type: "data-compaction", id: `${RUN_ID}-compaction`, data: { summarizedTurns: 6 } },
  { type: "reasoning-start", id: "r1" },
  { type: "reasoning-delta", id: "r1", delta: "Cần tìm " },
  { type: "reasoning-delta", id: "r1", delta: "chính sách." },
  { type: "reasoning-end", id: "r1", providerMetadata: { thomas: { durationMs: 900 } } },
  { type: "tool-input-start", toolCallId: "call-1", toolName: "kb_search", dynamic: true, toolMetadata: KB },
  {
    type: "tool-input-available",
    toolCallId: "call-1",
    toolName: "kb_search",
    dynamic: true,
    toolMetadata: KB,
    input: { query: "chính sách hoàn tiền" },
  },
  { type: "data-status", id: `${RUN_ID}-s1`, data: { label: "Đang tìm trong KB…" } },
  {
    type: "tool-output-available",
    toolCallId: "call-1",
    output: SEARCH_OUTPUT,
    dynamic: true,
    toolMetadata: { ...KB, durationMs: 640 },
  },
  { type: "tool-input-start", toolCallId: "call-2", toolName: "kb_read_section", dynamic: true, toolMetadata: KB },
  {
    type: "tool-input-available",
    toolCallId: "call-2",
    toolName: "kb_read_section",
    dynamic: true,
    toolMetadata: KB,
    input: { nodeId: "9b2f0c1e-0000-4000-8000-000000000001" },
  },
  {
    type: "tool-output-error",
    toolCallId: "call-2",
    errorText: "DEPENDENCY_TIMEOUT",
    dynamic: true,
    toolMetadata: { ...KB, durationMs: 20000 },
  },
  { type: "data-steer", id: `${RUN_ID}-steer-1`, data: { text: "Chỉ cần bảng giá" } },
  {
    type: "source-document",
    sourceId: "S1",
    mediaType: "application/pdf",
    title: "Chính sách › Hoàn tiền",
    filename: "policy.pdf",
    providerMetadata: {
      thomas: {
        fileId: "2b1e7c3a-0000-4000-8000-000000000002",
        kbId: "3c2f8d4b-0000-4000-8000-000000000003",
        fileName: "policy.pdf",
        kbName: "Chính sách",
        section: ["Chính sách", "Hoàn tiền"],
        pageStart: 3,
        pageEnd: 3,
        blockIds: ["b-1"],
        score: 0.91,
        locators: [{ kind: "pdf_bbox", page: 3, bbox: [10, 20, 300, 80] }],
      },
    },
  },
  { type: "text-start", id: "t1" },
  { type: "text-delta", id: "t1", delta: "Khách được hoàn tiền " },
  { type: "text-delta", id: "t1", delta: "trong 7 ngày [1]." },
  { type: "text-end", id: "t1" },
  {
    type: "data-citations",
    id: `${RUN_ID}-citations`,
    data: { flags: [{ sourceId: "S1", claim: "Khách được hoàn tiền trong 7 ngày", probability: 0.2 }] },
  },
  {
    type: "data-usage",
    id: `${RUN_ID}-usage`,
    data: {
      runId: RUN_ID,
      inputTokens: 1200,
      outputTokens: 80,
      costUsd: 0.0012,
      ttftMs: 700,
      durationMs: 3100,
      contextWindow: 128000,
    },
  },
  { type: "finish" },
];

const errorRun: Chunk[] = [
  { type: "start", messageId: MESSAGE_ID, messageMetadata: { runId: RUN_ID } },
  { type: "text-start", id: "t1" },
  { type: "text-delta", id: "t1", delta: "Đang trả lời" },
  { type: "error", errorText: "PROVIDER_UNAVAILABLE" },
];

async function capture(name: string, chunks: Chunk[], withMessage: boolean) {
  let message: UIMessage | undefined;
  const stream = createUIMessageStream<UIMessage>({
    originalMessages: [],
    execute: ({ writer }) => {
      for (const chunk of chunks) writer.write(chunk);
    },
    onEnd: ({ responseMessage }) => {
      message = responseMessage;
    },
  });
  const sse = await createUIMessageStreamResponse({ stream }).text();
  writeFileSync(join(OUT, `${name}.sse`), sse);
  if (withMessage) writeFileSync(join(OUT, `${name}.message.json`), `${JSON.stringify(message, null, 2)}\n`);
}

mkdirSync(OUT, { recursive: true });
await capture("kb-run", kbRun, true);
await capture("error", errorRun, false);
