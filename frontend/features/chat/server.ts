import "server-only";

import {
  DEFAULT_CHAT_ROLE,
  REGISTRY_PATHS,
  type Model,
  type ModelChoice,
  type RoleAssignment,
} from "@/features/registry/types";
import { serverFetchOptional as optional } from "@/lib/api/server";

import { CHAT_PATHS, type ChatScope, type ConversationDetail, type ConversationSummary } from "./contract";

export function listRecents(): Promise<ConversationSummary[]> {
  return optional(CHAT_PATHS.conversations, []);
}

export function getConversation(id: string): Promise<ConversationDetail | null> {
  return optional<ConversationDetail | null>(CHAT_PATHS.conversation(id), null);
}

export function getScope(): Promise<ChatScope | null> {
  return optional<ChatScope | null>(CHAT_PATHS.scope, null);
}

export async function getModelChoice(): Promise<ModelChoice> {
  const [models, roles] = await Promise.all([
    optional<Model[]>(REGISTRY_PATHS.chatModels, []),
    optional<RoleAssignment[]>(REGISTRY_PATHS.roles, []),
  ]);
  const defaultModelId = roles.find((r) => r.role === DEFAULT_CHAT_ROLE)?.modelId ?? models[0]?.id;
  return { models, defaultModelId };
}
