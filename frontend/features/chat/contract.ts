/**
 * PROVISIONAL M2 chat contract.
 *
 * Core does not serve chat yet; these shapes are what the web shell and the local fake core
 * (`mock-core/`) agree on today. When M2 writes the chat section of contracts/openapi/core.v1.yaml,
 * replace this file with the generated types and fix whatever no longer compiles.
 * Messages follow AI SDK UI v7 (`UIMessage`, `x-vercel-ai-ui-message-stream: v1`), per spec §7.2.
 */
import type { UIMessage } from "ai";

import type { Line, ToolLine } from "@/components/metro/lines";

export const CHAT_PATHS = {
  chat: "/api/v1/chat",
  scope: "/api/v1/chat/scope",
  conversations: "/api/v1/conversations",
  conversation: (id: string) => `/api/v1/conversations/${encodeURIComponent(id)}`,
  documentPage: (documentId: string, page: number) =>
    `/api/v1/documents/${encodeURIComponent(documentId)}/pages/${page}`,
} as const;

/** Namespace our runtime uses inside AI SDK `providerMetadata`. */
export const METADATA_NS = "thomas";

/** Carried on every tool part (`toolMetadata`): which line it runs on and how it is signed. */
export type ToolMeta = {
  line: ToolLine;
  /** The name plate: a KB name, an MCP server, a Composio toolkit or an App. */
  source: string;
  /** Present once the tool finished. */
  durationMs?: number;
};

/** `providerMetadata.thomas` on a finished reasoning part. */
export type ReasoningMeta = { durationMs: number };

/** `providerMetadata.thomas` on a `source-document` part: where the cited block lives. */
export type CitationLocator = {
  documentId: string;
  kbName: string;
  page: number;
  pageCount: number;
  section?: string;
  blockIds: string[];
  score: number;
};

export type RunUsage = {
  runId: string;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  ttftMs: number;
  durationMs: number;
};

export type ChatDataTypes = {
  /** Runtime progress label when the model streams no reasoning (spec §7.2). */
  status: { label: string };
  usage: RunUsage;
};

export type ChatMetadata = { runId?: string };

export type ChatMessage = UIMessage<ChatMetadata, ChatDataTypes>;
export type ChatPart = ChatMessage["parts"][number];

/** Built-in KB tools core registers for every Agent; the run line draws them with richer stations. */
export const KB_TOOLS = { search: "kb_search", readSection: "kb_read_section" } as const;

/** Outputs of the KB tools; other tools return free-form JSON the run line only summarises. */
export type KbSearchOutput = { chunks: number; documents: string[] };
export type KbSearchInput = { query: string };
export type KbReadInput = { section: string; fileName: string };

export type ConversationSummary = { id: string; title: string; lines: ToolLine[]; updatedAt: string };

export type ChatScope = {
  agent: { id: string; name: string };
  kb: { id: string; name: string };
  toolSources: Array<{ line: Line; name: string }>;
};

export type ConversationDetail = ConversationSummary & {
  folder?: string;
  scope: ChatScope;
  messages: ChatMessage[];
};

export type DocumentBlock = { id: string; text: string; kind: "heading" | "paragraph" };

export type DocumentPage = {
  documentId: string;
  fileName: string;
  kbName: string;
  page: number;
  pageCount: number;
  section?: string;
  version?: string;
  blocks: DocumentBlock[];
};

/** Body our transport sends: only the newest message; core rebuilds history itself (spec §7.1). */
export type ChatRequestBody = {
  id: string;
  trigger: "submit-message" | "regenerate-message";
  messageId?: string;
  message: ChatMessage;
  modelId?: string;
};
