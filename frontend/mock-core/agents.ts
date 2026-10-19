import { createHash, randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import type { Agent, AgentCreate, AgentPatch } from "../lib/core/types.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { agentSeeds } from "./fixtures/agents.ts";
import { readJson, sendData, sendError, sendFieldError, sendNoContent } from "./http.ts";
import { knowledgeBasesOf } from "./knowledge-bases.ts";

const HOUR_MS = 3_600_000;
const HTTP_CREATED = 201;
const stores = new Map<string, Map<string, Agent>>();

const UUID_HEX_DIGITS = 32;
const UUID_GROUPS = /^(.{8})(.{4})(.{4})(.{4})(.{12})$/;

/** A seeded Agent keeps its id across `/__mock/reset`, so a page opened before a reset still finds it. */
function seedId(orgId: string, index: number): string {
  const hex = createHash("sha256").update(`${orgId}:agent:${index}`).digest("hex");
  return hex.slice(0, UUID_HEX_DIGITS).replace(UUID_GROUPS, "$1-$2-$3-$4-$5");
}

/** One seeded Agent of an org, or undefined when the org has no KB to bind it to. */
function seedAgent(orgId: string, index: number): Agent | undefined {
  const seed = agentSeeds[index];
  const kbs = knowledgeBasesOf(orgId);
  const kb = kbs[seed?.kbIndex ?? 0] ?? kbs[0];
  if (!seed || !kb) return undefined;
  const at = new Date(Date.now() - seed.hoursAgo * HOUR_MS).toISOString();
  return {
    id: seedId(orgId, index),
    name: seed.name,
    description: seed.description,
    instructions: seed.instructions,
    kbId: kb.id,
    kbName: kb.name,
    kbMissing: false,
    modelOverrides: {},
    createdAt: at,
    updatedAt: at,
  };
}

function seeded(orgId: string): Map<string, Agent> {
  const existing = stores.get(orgId);
  if (existing) return existing;
  const store = new Map<string, Agent>();
  for (const index of agentSeeds.keys()) {
    const agent = seedAgent(orgId, index);
    if (agent) store.set(agent.id, agent);
  }
  stores.set(orgId, store);
  return store;
}

/**
 * `POST /__mock/reset`: forget every org's Agents (they reseed on the next read), or with `name` restore only the
 * seeded Agent of that name in every org. The Agents e2e resets per test through the scoped form, because the
 * desktop and mobile projects edit different Agents and other specs use the same fake in parallel.
 */
export function resetAgents(name?: string) {
  if (!name) return stores.clear();
  for (const [orgId, store] of stores) {
    const index = agentSeeds.findIndex((seed) => seed.name === name);
    const agent = seedAgent(orgId, index);
    if (agent) store.set(agent.id, agent);
  }
}

/** Like core: the KB name is read from ragx at read time, and a KB ragx no longer has is `kbMissing`. */
function withKb(orgId: string, agent: Agent): Agent {
  const kb = knowledgeBasesOf(orgId).find((k) => k.id === agent.kbId);
  return { ...agent, kbName: kb?.name ?? "", kbMissing: !kb };
}

export function listAgentsOf(orgId: string): Agent[] {
  return [...seeded(orgId).values()]
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
    .map((a) => withKb(orgId, a));
}

export function listAgents(_req: IncomingMessage, res: ServerResponse, orgId: string) {
  sendData(res, listAgentsOf(orgId));
}

export function getAgent(req: IncomingMessage, res: ServerResponse, orgId: string, id: string) {
  const agent = seeded(orgId).get(id);
  return agent ? sendData(res, withKb(orgId, agent)) : sendError(req, res, ErrorCode.AGENT_NOT_FOUND);
}

const kbIsTheOrgs = (orgId: string, kbId: string | undefined) =>
  !!kbId && knowledgeBasesOf(orgId).some((k) => k.id === kbId);

/** Core's create checks, in its order; answers the refusal itself and returns nothing for a refused body. */
function acceptedCreate(
  req: IncomingMessage,
  res: ServerResponse,
  orgId: string,
  body: AgentCreate | undefined,
): AgentCreate | undefined {
  const name = body?.name?.trim() ?? "";
  if (!body || !name) {
    sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [{ field: "name", code: "REQUIRED" }]);
    return undefined;
  }
  if (!kbIsTheOrgs(orgId, body.kbId)) {
    sendError(req, res, ErrorCode.AGENT_KB_INVALID);
    return undefined;
  }
  if ([...seeded(orgId).values()].some((a) => a.name === name)) {
    sendFieldError(req, res, ErrorCode.CONFLICT, [{ field: "name", code: "TAKEN" }]);
    return undefined;
  }
  return { ...body, name };
}

export async function createAgent(req: IncomingMessage, res: ServerResponse, orgId: string) {
  const input = acceptedCreate(req, res, orgId, await readJson<AgentCreate>(req));
  if (!input) return;
  const now = new Date().toISOString();
  const agent: Agent = {
    id: randomUUID(),
    name: input.name,
    description: input.description ?? "",
    instructions: input.instructions ?? "",
    kbId: input.kbId,
    kbName: "",
    kbMissing: false,
    modelOverrides: {},
    createdAt: now,
    updatedAt: now,
  };
  seeded(orgId).set(agent.id, agent);
  sendData(res, withKb(orgId, agent), HTTP_CREATED);
}

/** `null` clears a role's pin, a model id sets it, an absent role keeps what is stored. */
function mergeOverrides(stored: Agent["modelOverrides"], change: AgentPatch["modelOverrides"]) {
  const merged: Record<string, string | null | undefined> = { ...stored, ...change };
  return Object.fromEntries(
    Object.entries(merged).filter(([, modelId]) => typeof modelId === "string"),
  ) as Agent["modelOverrides"];
}

function applyPatch(current: Agent, patch: AgentPatch): Agent {
  return {
    ...current,
    ...(patch.name !== undefined && { name: patch.name }),
    ...(patch.description !== undefined && { description: patch.description }),
    ...(patch.instructions !== undefined && { instructions: patch.instructions }),
    ...(patch.kbId !== undefined && { kbId: patch.kbId }),
    modelOverrides: mergeOverrides(current.modelOverrides, patch.modelOverrides),
    updatedAt: new Date().toISOString(),
  };
}

export async function patchAgent(req: IncomingMessage, res: ServerResponse, orgId: string, id: string) {
  const store = seeded(orgId);
  const current = store.get(id);
  if (!current) return sendError(req, res, ErrorCode.AGENT_NOT_FOUND);
  const patch = (await readJson<AgentPatch>(req)) ?? {};
  if (patch.kbId !== undefined && !kbIsTheOrgs(orgId, patch.kbId))
    return sendError(req, res, ErrorCode.AGENT_KB_INVALID);
  const next = applyPatch(current, patch);
  store.set(id, next);
  sendData(res, withKb(orgId, next));
}

export function deleteAgent(req: IncomingMessage, res: ServerResponse, orgId: string, id: string) {
  if (!seeded(orgId).delete(id)) return sendError(req, res, ErrorCode.AGENT_NOT_FOUND);
  sendNoContent(res);
}
