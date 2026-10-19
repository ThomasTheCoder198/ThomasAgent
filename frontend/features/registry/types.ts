/** Mirrors the registry schemas in contracts/openapi/core.v1.yaml (M0.2). */
export type Capability = "chat" | "tools" | "vision" | "reasoning" | "embedding" | "rerank" | "decision";
export type Role = "chat.default" | "chat.fast" | "vision" | "embedding" | "rerank" | "decision";

export type Model = {
  id: string;
  providerId: string;
  modelRef: string;
  displayName: string;
  capabilities: Capability[];
  /** O8 leaves most windows unknown, so null is a normal value. */
  contextWindow: number | null;
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

export type ProviderKind =
  | "openrouter"
  | "openai"
  | "anthropic"
  | "google"
  | "azure"
  | "ollama"
  | "openai_compatible"
  | "cohere"
  | "voyage"
  | "jina"
  | "typesafe";

/** Mirrors `Provider` in core.v1.yaml (hand-written like the other M0.2 registry types, C2-2). */
export type Provider = {
  id: string;
  kind: ProviderKind;
  name: string;
  baseUrl: string;
  hasApiKey: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

/** `apiKey`: absent = unchanged (cleared when `baseUrl` changes), empty string = remove. */
export type ProviderInput = {
  kind?: ProviderKind;
  name?: string;
  baseUrl?: string;
  apiKey?: string | null;
  enabled?: boolean;
};

export const PROVIDER_PATHS = {
  providers: "/api/v1/providers",
  provider: (id: string) => `/api/v1/providers/${encodeURIComponent(id)}`,
  sync: (id: string) => `/api/v1/providers/${encodeURIComponent(id)}/sync`,
  role: (role: Role) => `/api/v1/model-roles/${encodeURIComponent(role)}`,
  modelTest: (id: string) => `/api/v1/models/${encodeURIComponent(id)}/test`,
  model: (id: string) => `/api/v1/models/${encodeURIComponent(id)}`,
} as const;
