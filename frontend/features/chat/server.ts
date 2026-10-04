import "server-only";

import { ApiError } from "@/lib/api/client";
import { serverFetch } from "@/lib/api/server";
import { ErrorCode } from "@/lib/errors/codes.gen";
import {
  DEFAULT_CHAT_ROLE,
  REGISTRY_PATHS,
  type Model,
  type ModelChoice,
  type RoleAssignment,
} from "@/features/registry/types";

import { CHAT_PATHS, type ChatScope, type ConversationDetail, type ConversationSummary } from "./contract";

/** Until core serves an endpoint (M2), treat its 404 as "nothing yet" instead of failing the shell. */
async function optional<T>(path: string, fallback: T): Promise<T> {
  try {
    return await serverFetch<T>(path);
  } catch (err) {
    if (err instanceof ApiError && err.code === ErrorCode.NOT_FOUND) return fallback;
    throw err;
  }
}

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
