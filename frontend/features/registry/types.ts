/** Mirrors the registry schemas in contracts/openapi/core.v1.yaml (M0.2). */
export type Capability = "chat" | "tools" | "vision" | "reasoning" | "embedding" | "rerank" | "decision";
export type Role = "chat.default" | "chat.fast" | "vision" | "embedding" | "rerank" | "decision";

export type Model = {
  id: string;
  providerId: string;
  modelRef: string;
  displayName: string;
  capabilities: Capability[];
  contextWindow: number;
  embeddingDims?: number;
  inputPricePerMTok?: number;
  outputPricePerMTok?: number;
  source: "manual" | "sync";
};

export type RoleAssignment = { role: Role; modelId: string };

/** Chat-capable models plus the one the `chat.default` role points at. */
export type ModelChoice = { models: Model[]; defaultModelId: string | undefined };

export const REGISTRY_PATHS = {
  models: "/api/v1/models",
  chatModels: "/api/v1/models?capability=chat",
  roles: "/api/v1/model-roles",
} as const;

export const DEFAULT_CHAT_ROLE: Role = "chat.default";
