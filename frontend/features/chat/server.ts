import "server-only";

import { REGISTRY_PATHS, type Model, type ModelChoice } from "@/features/registry/types";
import { ORG_ROLE_PATHS } from "@/features/settings/contract";
import { serverFetchOptional as optional } from "@/lib/api/server";
import type { OrgModelRole } from "@/lib/core/types";

import { CHAT_PATHS, type ConversationDetail, type ConversationSummary } from "./contract";

const DEFAULT_CHAT_ROLE = "chat.default";

export function listRecents(orgId: string): Promise<ConversationSummary[]> {
  return optional(CHAT_PATHS.conversations(orgId), []);
}

/** null when the org has no such conversation; another org's id answers not-found too (R1). */
export function getConversation(orgId: string, id: string): Promise<ConversationDetail | null> {
  return optional<ConversationDetail | null>(CHAT_PATHS.conversation(orgId, id), null);
}

/** Chat models, defaulting to what `chat.default` resolves to for this org (its override, else the platform's). */
export async function getModelChoice(orgId: string): Promise<ModelChoice> {
  const [models, roles] = await Promise.all([
    optional<Model[]>(REGISTRY_PATHS.chatModels, []),
    optional<OrgModelRole[]>(ORG_ROLE_PATHS.roles(orgId), []),
  ]);
  // No fallback to the first listed model: it may lack tools, and it is not the operator's choice. Unset means
  // the composer asks for an explicit pick (Codex review #8).
  const effective = roles.find((r) => r.role === DEFAULT_CHAT_ROLE)?.effectiveModelId ?? undefined;
  return { models, defaultModelId: effective };
}
