/**
 * The chat contract between the web and core (contracts/openapi/core.v1.yaml, tag Chat).
 * Wire types are aliases of the generated ones in `lib/core/types.ts`; this file adds only paths, constants and
 * the AI SDK message typing. Messages follow AI SDK UI v7 (`UIMessage`, `x-vercel-ai-ui-message-stream: v1`),
 * spec §7.2. Every path is org-scoped (index R1).
 */
import type { UIMessage } from "ai";

import type { ToolLine } from "@/components/lines";
import type {
  ChatRequest,
  ChatScope as ChatScopeWire,
  ConversationDetail as ConversationDetailWire,
  ConversationSummary as ConversationSummaryWire,
  RunUsage as RunUsageWire,
} from "@/lib/core/types";

const enc = encodeURIComponent;
const org = (orgId: string) => `/api/v1/orgs/${enc(orgId)}`;

export const CHAT_PATHS = {
  chat: (orgId: string) => `${org(orgId)}/chat`,
  steer: (orgId: string, runId: string) => `${org(orgId)}/runs/${enc(runId)}/steer`,
  conversations: (orgId: string) => `${org(orgId)}/conversations`,
  conversation: (orgId: string, id: string) => `${org(orgId)}/conversations/${enc(id)}`,
  // Removed in Task 2c-4, when the Evidence Viewer reads ragx's file preview instead.
  documentPage: (documentId: string, page: number) => `/api/v1/documents/${enc(documentId)}/pages/${page}`,
} as const;

/** Namespace our runtime uses inside AI SDK `providerMetadata`; `toolMetadata` is unnamespaced (M2b Q5, the mock). */
export const METADATA_NS = "thomas";

/** The KB tools core gives every Agent (ragx manifest); the run line draws them with richer stations. */
export const KB_TOOL_NAMES = ["kb_search", "kb_outline", "kb_read_section", "kb_list_documents"] as const;
export type KbToolName = (typeof KB_TOOL_NAMES)[number];
export const KB_TOOLS = { search: "kb_search", readSection: "kb_read_section" } as const;

/** `source-document` ids are `S{n}`: core numbers sources run-wide (index R10, ruling C2-4). */
export const SOURCE_ID_PATTERN = /^S(\d+)$/;

/** Mirror core's caps (index Global Constraints) so the composer refuses early; core stays the authority. */
export const MESSAGE_MAX_RUNES = 8000;
export const STEER_MAX_RUNES = 2000;

/** Carried on every tool part (`toolMetadata`, unnamespaced): which line it runs on and how it is signed. */
export type ToolMeta = {
  line: ToolLine;
  /** The name plate: a KB name, an MCP server, a Composio toolkit or an App. */
  source: string;
  /** Present once the tool finished. */
  durationMs?: number;
};

/** `providerMetadata.thomas` on a finished reasoning part. */
export type ReasoningMeta = { durationMs: number };

/**
 * PROVISIONAL until Task 2c-4: the page-based locator the current Evidence Viewer draws. Task 2c-4 replaces it
 * with `CitationLocatorWire` (file, block ids, ragx locators).
 */
export type CitationLocator = {
  documentId: string;
  kbName: string;
  page: number;
  pageCount: number;
  section?: string;
  blockIds: string[];
  score: number;
};

export type RunUsage = RunUsageWire;
export type CitationFlag = { sourceId: string; claim: string; probability: number };

export type ChatDataTypes = {
  /** Runtime progress label when the model streams no reasoning (spec §7.2). */
  status: { label: string };
  usage: RunUsage;
  /** O1: older turns were summarized before this run. */
  compaction: { summarizedTurns: number };
  /** O4: claims whose cited evidence may not support them. */
  citations: { flags: CitationFlag[] };
  /** R17: an instruction the operator sent while the run streamed. */
  steer: { text: string };
};

export type ChatMetadata = { runId?: string };

export type ChatMessage = UIMessage<ChatMetadata, ChatDataTypes>;
export type ChatPart = ChatMessage["parts"][number];

export type ConversationSummary = ConversationSummaryWire;
export type ChatScope = ChatScopeWire;
/** The wire detail carries messages as free-form JSON; they are AI SDK messages in exactly the stored shape. */
export type ConversationDetail = Omit<ConversationDetailWire, "messages"> & { messages: ChatMessage[] };

export type DocumentBlock = { id: string; text: string; kind: "heading" | "paragraph" };

/** PROVISIONAL until Task 2c-4 (see `CitationLocator`). */
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

/** The body our transport sends: only the newest message; core rebuilds history itself (spec §7.1). */
export type ChatRequestBody = Omit<ChatRequest, "message"> & { message: ChatMessage };
