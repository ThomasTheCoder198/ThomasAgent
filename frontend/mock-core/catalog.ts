import { randomUUID } from "node:crypto";

import type { Model, Provider, Role, RoleAssignment } from "../features/registry/types.ts";

/** SYNTHETIC registry rows in the documented M0.2 shapes; the real ones come from the owner's providers. */
const providerId = randomUUID();
const selfHostedId = randomUUID();
/** Fixed for the process so a page opened before a reset still finds its models. */
const modelIds = [randomUUID(), randomUUID(), randomUUID(), randomUUID()] as const;
const SEEDED_AT = "2026-10-10T08:00:00Z";

const seededProviders = (): Provider[] => [
  {
    id: providerId,
    kind: "openrouter",
    name: "OpenRouter",
    baseUrl: "https://openrouter.ai/api/v1",
    hasApiKey: true,
    enabled: true,
    createdAt: SEEDED_AT,
    updatedAt: SEEDED_AT,
  },
  {
    id: selfHostedId,
    kind: "openai_compatible",
    name: "vLLM nội bộ",
    baseUrl: "http://localhost:8000/v1",
    hasApiKey: false,
    enabled: true,
    createdAt: SEEDED_AT,
    updatedAt: SEEDED_AT,
  },
];

const seededModels = (): Model[] => [
  {
    id: modelIds[0],
    providerId,
    modelRef: "anthropic/claude-sonnet-5.5",
    displayName: "Claude Sonnet 5.5",
    capabilities: ["chat", "tools", "reasoning", "vision"],
    contextWindow: 200_000,
    inputPricePerMTok: 3,
    outputPricePerMTok: 15,
    source: "sync",
  },
  {
    id: modelIds[1],
    providerId,
    modelRef: "openai/gpt-5-mini",
    displayName: "GPT-5 mini",
    capabilities: ["chat", "tools"],
    contextWindow: 128_000,
    inputPricePerMTok: 0.25,
    outputPricePerMTok: 2,
    source: "sync",
  },
  // A decision model (Jev) and one whose provider reported nothing (O8): neither is a chat model, so the chat
  // and Agent pickers stay as they were, while Settings shows both cases.
  {
    id: modelIds[2],
    providerId,
    modelRef: "typesafe/jev-1.14",
    displayName: "Jev 1.14",
    capabilities: ["decision"],
    contextWindow: 32_000,
    inputPricePerMTok: 0.05,
    outputPricePerMTok: 0.1,
    source: "sync",
  },
  {
    id: modelIds[3],
    providerId,
    modelRef: "meta-llama/llama-3.3-70b-instruct",
    displayName: "Llama 3.3 70B",
    capabilities: [],
    contextWindow: null,
    source: "sync",
  },
];

const seededRoles = (): RoleAssignment[] => [
  { role: "chat.default", modelId: modelIds[0] },
  { role: "chat.fast", modelId: modelIds[1] },
];

export const providers: Provider[] = seededProviders();
export const models: Model[] = seededModels();
export const roles: RoleAssignment[] = seededRoles();
/** orgId → role → the org's own model. */
export const orgRoles = new Map<string, Map<Role, string>>();

function replaceAll<T>(target: T[], next: T[]) {
  target.splice(0, target.length, ...next);
}

/** Joins `POST /__mock/reset`: providers, models and every role assignment return to the seeded state. */
export function resetCatalog() {
  replaceAll(providers, seededProviders());
  replaceAll(models, seededModels());
  replaceAll(roles, seededRoles());
  orgRoles.clear();
}

export function setRole(role: Role, modelId: string) {
  const existing = roles.find((r) => r.role === role);
  if (existing) existing.modelId = modelId;
  else roles.push({ role, modelId });
}

export function clearRole(role: Role) {
  replaceAll(
    roles,
    roles.filter((r) => r.role !== role),
  );
}

export function setOrgRole(orgId: string, role: Role, modelId: string) {
  const own = orgRoles.get(orgId) ?? new Map<Role, string>();
  own.set(role, modelId);
  orgRoles.set(orgId, own);
}

export function clearOrgRole(orgId: string, role: Role) {
  orgRoles.get(orgId)?.delete(role);
}
