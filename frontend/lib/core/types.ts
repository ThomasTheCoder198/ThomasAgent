/** Core's M2 API types, generated from contracts/openapi/core.v1.yaml by `task gen` (rule 3: never hand-written). */
import type { components } from "./api.gen";
import type { components as ragx } from "../ragx/api.gen";

type Schemas = components["schemas"];

export type Agent = Schemas["Agent"];
export type AgentCreate = Schemas["AgentCreate"];
export type AgentPatch = Schemas["AgentPatch"];
export type ConversationSummary = Schemas["ConversationSummary"];
export type ConversationDetail = Schemas["ConversationDetail"];
export type RunSummary = Schemas["RunSummary"];
export type ChatScope = Schemas["ChatScope"];
export type OrgModelRole = Schemas["OrgModelRole"];
export type ModelTestResult = Schemas["ModelTestResult"];

// PROVISIONAL (F2c1-1): replace with generated aliases when 2b-4/2b-5 add these schemas to core.v1.yaml.
// Shapes follow global-constraints-index.md "Wire contracts shared by core and web" and index amendment 7.

/** index "Wire contracts", `POST /api/v1/orgs/{orgId}/chat` request. */
export type ChatRequest = {
  id: string;
  /** Required when the conversation is new (R12); core ignores or rejects it for a stored one. */
  agentId?: string;
  trigger: "submit-message" | "regenerate-message";
  /** Regenerate only. */
  messageId?: string;
  message: Schemas["UIMessage"];
  modelId?: string;
};

/** index "Wire contracts", `POST /api/v1/orgs/{orgId}/runs/{runId}/steer` request. */
export type SteerRequest = { text: string };

/** index "Wire contracts" `data-usage` plus amendment 7 (`contextWindow`, the effective window used for O1). */
export type RunUsage = {
  runId: string;
  inputTokens: number;
  outputTokens: number;
  costUsd: number | null;
  ttftMs: number;
  durationMs: number;
  contextWindow: number;
};

/** index "Wire contracts", `CitationLocator` (`providerMetadata.thomas` of `source-document`); `Locator` is ragx's. */
export type CitationLocatorWire = {
  fileId: string;
  kbId: string;
  fileName: string;
  fileVersion?: number;
  kbName: string;
  section: string[];
  pageStart: number | null;
  pageEnd: number | null;
  blockIds: string[];
  score: number;
  locators: ragx["schemas"]["Locator"][];
};
