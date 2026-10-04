import type { IncomingMessage, ServerResponse } from "node:http";

import {
  agentRoles,
  toolPolicies,
  type AgentDetail,
  type AgentPatch,
  type AgentSummary,
} from "../features/agents/contract.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { models, roles } from "./catalog.ts";
import { agents, foreignKnowledgeBases, knowledgeBases } from "./fixtures/agents.ts";
import { readJson, sendData, sendError } from "./http.ts";

/** The fixture uses "fast" as a stand-in for "whatever chat.fast points at"; resolve it to a real model id. */
const ROLE_ALIAS = "fast";

function resolveOverrides(agent: AgentDetail): AgentDetail {
  const overrides = { ...agent.modelOverrides };
  for (const role of agentRoles) {
    if (overrides[role] === ROLE_ALIAS) overrides[role] = roles.find((r) => r.role === "chat.fast")?.modelId;
  }
  return { ...agent, modelOverrides: overrides };
}

const store = new Map<string, AgentDetail>();

/**
 * Restores seeded Agents — all of them, or just `id` so parallel e2e workers each reset only the Agent they
 * edit. Called through `POST /__mock/reset[?agent=id]`.
 */
export function resetAgents(id?: string) {
  if (!id) store.clear();
  for (const agent of agents) {
    if (!id || agent.id === id) store.set(agent.id, resolveOverrides(agent));
  }
}

resetAgents();

function summarize(agent: AgentDetail): AgentSummary {
  const kb = knowledgeBases.find((k) => k.id === agent.kbId);
  const chatModelId =
    agent.modelOverrides["chat.default"] ?? roles.find((r) => r.role === "chat.default")?.modelId;
  const enabled = agent.toolSources.filter((s) => s.tools.some((t) => t.policy !== "off"));
  return {
    id: agent.id,
    name: agent.name,
    description: agent.description,
    kb: { id: agent.kbId, name: kb?.name ?? agent.kbId },
    lines: [...new Set(enabled.map((s) => s.line))],
    toolCount: agent.toolSources.flatMap((s) => s.tools).filter((t) => t.policy !== "off").length,
    chatModel: models.find((m) => m.id === chatModelId)?.displayName ?? "",
    updatedAt: agent.updatedAt,
  };
}

export function listAgents(_req: IncomingMessage, res: ServerResponse) {
  sendData(res, [...store.values()].map(summarize));
}

export function listKnowledgeBases(_req: IncomingMessage, res: ServerResponse) {
  sendData(res, knowledgeBases);
}

export function getAgent(req: IncomingMessage, res: ServerResponse, id: string) {
  const agent = store.get(id);
  return agent ? sendData(res, agent) : sendError(req, res, ErrorCode.NOT_FOUND);
}

type PatchVerdict = "ok" | "invalid" | "forbidden";

/**
 * Mirrors what core must do (AGENTS rule 11): the client's kbId and model ids are never trusted. A KB that
 * belongs to another tenant is forbidden; an id that does not exist at all is invalid.
 */
function checkPatch(patch: AgentPatch): PatchVerdict {
  if (patch.name !== undefined && patch.name.trim().length === 0) return "invalid";
  if (patch.kbId !== undefined && !knowledgeBases.some((k) => k.id === patch.kbId)) {
    return foreignKnowledgeBases.some((k) => k.id === patch.kbId) ? "forbidden" : "invalid";
  }
  const unknownModel = Object.values(patch.modelOverrides ?? {}).some(
    (id) => id !== null && !models.some((m) => m.id === id),
  );
  if (unknownModel) return "invalid";
  const policies = Object.values(patch.toolPolicies ?? {});
  return policies.every((p) => toolPolicies.includes(p)) ? "ok" : "invalid";
}

function applyPatch(agent: AgentDetail, patch: AgentPatch): AgentDetail {
  const merged = { ...agent.modelOverrides, ...patch.modelOverrides };
  // `null` in the patch clears an override, so drop those roles instead of storing null.
  const modelOverrides = Object.fromEntries(Object.entries(merged).filter(([, modelId]) => modelId !== null));
  const toolSources = agent.toolSources.map((source) => ({
    ...source,
    tools: source.tools.map((tool) => ({ ...tool, policy: patch.toolPolicies?.[tool.id] ?? tool.policy })),
  }));
  const { name, description, instructions, kbId } = patch;
  return {
    ...agent,
    ...(name !== undefined && { name: name.trim() }),
    ...(description !== undefined && { description }),
    ...(instructions !== undefined && { instructions }),
    ...(kbId !== undefined && { kbId }),
    toolSources,
    modelOverrides,
    updatedAt: new Date().toISOString(),
  };
}

export async function patchAgent(req: IncomingMessage, res: ServerResponse, id: string) {
  const agent = store.get(id);
  if (!agent) return sendError(req, res, ErrorCode.NOT_FOUND);
  const patch = await readJson<AgentPatch>(req);
  const verdict = patch ? checkPatch(patch) : "invalid";
  if (verdict === "forbidden") return sendError(req, res, ErrorCode.FORBIDDEN);
  if (!patch || verdict === "invalid") return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  const next = applyPatch(agent, patch);
  store.set(id, next);
  return sendData(res, next);
}
