import { randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import type {
  Capability,
  Model,
  Provider,
  ProviderInput,
  ProviderKind,
  Role,
} from "../features/registry/types.ts";
import { fitsRole, SETTINGS_ROLES, type SettingsRole } from "../features/settings/role-fit.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import {
  clearOrgRole,
  clearRole,
  models,
  orgRoles,
  providers,
  roles,
  setOrgRole,
  setRole,
} from "./catalog.ts";
import { readJson, sendData, sendError, sendFieldError, sendNoContent } from "./http.ts";
import { findOrg } from "./orgs.ts";

const HTTP_CREATED = 201;
const MODEL_TEST_LATENCY_MS = 412;
const NEEDS_BASE_URL: readonly ProviderKind[] = ["azure", "ollama", "openai_compatible"];
const DEFAULT_BASE_URLS: Partial<Record<ProviderKind, string>> = {
  openrouter: "https://openrouter.ai/api/v1",
  openai: "https://api.openai.com/v1",
  anthropic: "https://api.anthropic.com",
  google: "https://generativelanguage.googleapis.com",
};

const isSettingsRole = (role: string): role is SettingsRole =>
  (SETTINGS_ROLES as readonly string[]).includes(role);
const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();
const isUrl = (value: string) => URL.canParse(value);

type Reply = { req: IncomingMessage; res: ServerResponse };

/** GET /models honours `capability` and `providerId` like core; the chat pickers ask for `capability=chat`. */
export function listModels(req: IncomingMessage, res: ServerResponse) {
  const params = new URL(req.url ?? "/", "http://mock-core").searchParams;
  const capability = params.get("capability");
  const provider = params.get("providerId");
  sendData(
    res,
    models.filter(
      (m) =>
        (!capability || m.capabilities.includes(capability as Capability)) &&
        (!provider || m.providerId === provider),
    ),
  );
}

export const listProviders = (_req: IncomingMessage, res: ServerResponse) => sendData(res, providers);

function validateProvider(input: ProviderInput, kind: ProviderKind): { field: string; code: string }[] {
  const fields: { field: string; code: string }[] = [];
  const baseUrl = input.baseUrl ?? "";
  if (NEEDS_BASE_URL.includes(kind) && baseUrl === "") fields.push({ field: "baseUrl", code: "REQUIRED" });
  else if (baseUrl !== "" && !isUrl(baseUrl)) fields.push({ field: "baseUrl", code: "INVALID" });
  return fields;
}

export async function createProvider(req: IncomingMessage, res: ServerResponse) {
  const input = await readJson<ProviderInput>(req);
  if (!input?.kind || !input.name?.trim())
    return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [{ field: "name", code: "REQUIRED" }]);
  const invalid = validateProvider(input, input.kind);
  if (invalid.length > 0) return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, invalid);
  if (providers.some((p) => sameName(p.name, input.name ?? "")))
    return sendError(req, res, ErrorCode.LLM_NAME_TAKEN);
  const now = new Date().toISOString();
  const created: Provider = {
    id: randomUUID(),
    kind: input.kind,
    name: input.name.trim(),
    baseUrl: input.baseUrl ?? DEFAULT_BASE_URLS[input.kind] ?? "",
    hasApiKey: !!input.apiKey,
    enabled: input.enabled ?? true,
    createdAt: now,
    updatedAt: now,
  };
  providers.push(created);
  sendData(res, created, HTTP_CREATED);
}

/** Like core: a new address without a new key drops the old key; an empty key removes it. */
function applyProviderPatch(found: Provider, input: ProviderInput) {
  const addressChanged = input.baseUrl !== undefined && input.baseUrl !== found.baseUrl;
  if (input.name) found.name = input.name.trim();
  if (input.baseUrl !== undefined) found.baseUrl = input.baseUrl;
  if (input.enabled !== undefined) found.enabled = input.enabled;
  if (input.apiKey) found.hasApiKey = true;
  else if (input.apiKey === "" || addressChanged) found.hasApiKey = false;
  found.updatedAt = new Date().toISOString();
}

export async function patchProvider(req: IncomingMessage, res: ServerResponse, id: string) {
  const found = providers.find((p) => p.id === id);
  if (!found) return sendError(req, res, ErrorCode.LLM_PROVIDER_NOT_FOUND);
  const input = (await readJson<ProviderInput>(req)) ?? {};
  const invalid = validateProvider({ ...input, baseUrl: input.baseUrl ?? found.baseUrl }, found.kind);
  if (invalid.length > 0) return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, invalid);
  if (input.name && providers.some((p) => p.id !== id && sameName(p.name, input.name ?? ""))) {
    return sendError(req, res, ErrorCode.LLM_NAME_TAKEN);
  }
  applyProviderPatch(found, input);
  sendData(res, found);
}

const roleUses = (modelId: string) =>
  roles.some((r) => r.modelId === modelId) ||
  [...orgRoles.values()].some((own) => [...own.values()].includes(modelId));

export function deleteProvider({ req, res }: Reply, id: string) {
  const index = providers.findIndex((p) => p.id === id);
  if (index < 0) return sendError(req, res, ErrorCode.LLM_PROVIDER_NOT_FOUND);
  const own = models.filter((m) => m.providerId === id);
  if (own.some((m) => roleUses(m.id))) return sendError(req, res, ErrorCode.LLM_MODEL_IN_USE);
  providers.splice(index, 1);
  for (const m of own) models.splice(models.indexOf(m), 1);
  sendData(res, { deleted: true });
}

export function syncProvider({ req, res }: Reply, id: string) {
  if (!providers.some((p) => p.id === id)) return sendError(req, res, ErrorCode.LLM_PROVIDER_NOT_FOUND);
  sendData(res, { added: 0, updated: models.filter((m) => m.providerId === id).length, skipped: 0 });
}

export function testModel({ req, res }: Reply, id: string) {
  if (!models.some((m) => m.id === id)) return sendError(req, res, ErrorCode.LLM_MODEL_NOT_FOUND);
  sendData(res, { ok: true, latencyMs: MODEL_TEST_LATENCY_MS });
}

export async function patchModel(req: IncomingMessage, res: ServerResponse, id: string) {
  const found = models.find((m) => m.id === id);
  if (!found) return sendError(req, res, ErrorCode.LLM_MODEL_NOT_FOUND);
  const body = await readJson<{ capabilities?: Capability[] }>(req);
  if (!body?.capabilities?.length)
    return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [
      { field: "capabilities", code: "REQUIRED" },
    ]);
  const next: Model = { ...found, capabilities: body.capabilities };
  // Core refuses an edit that would leave a role pinned to a model without what the role needs.
  const pinned = roles.filter((r) => r.modelId === id && isSettingsRole(r.role));
  if (pinned.some((r) => !fitsRole(next, r.role as SettingsRole))) {
    return sendError(req, res, ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH);
  }
  found.capabilities = next.capabilities;
  sendData(res, found);
}

export async function putRole(req: IncomingMessage, res: ServerResponse, role: string, orgId?: string) {
  if (!isSettingsRole(role))
    return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [{ field: "role", code: "INVALID" }]);
  if (orgId !== undefined && !findOrg(orgId)) return sendError(req, res, ErrorCode.NOT_FOUND);
  const body = await readJson<{ modelId?: string }>(req);
  const model = models.find((m) => m.id === body?.modelId);
  if (!model) return sendError(req, res, ErrorCode.LLM_MODEL_NOT_FOUND);
  if (!fitsRole(model, role)) return sendError(req, res, ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH);
  if (orgId === undefined) setRole(role, model.id);
  else setOrgRole(orgId, role, model.id);
  sendData(res, { role, modelId: model.id });
}

export function deleteRole({ req, res }: Reply, role: string, orgId?: string) {
  if (!isSettingsRole(role))
    return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [{ field: "role", code: "INVALID" }]);
  if (orgId !== undefined && !findOrg(orgId)) return sendError(req, res, ErrorCode.NOT_FOUND);
  if (orgId === undefined) clearRole(role);
  else clearOrgRole(orgId, role);
  sendNoContent(res);
}

/** What `GET /orgs/{orgId}/model-roles` answers: the org's override, else the platform's, else the decision fallback. */
export function listOrgRoles({ req, res }: Reply, orgId: string) {
  if (!findOrg(orgId)) return sendError(req, res, ErrorCode.NOT_FOUND);
  const own = orgRoles.get(orgId);
  const platform = (role: Role) => roles.find((r) => r.role === role)?.modelId ?? null;
  const effective = (role: Role) => own?.get(role) ?? platform(role);
  sendData(
    res,
    SETTINGS_ROLES.map((role) => {
      const ownId = own?.get(role) ?? null;
      if (ownId) return { role, modelId: ownId, effectiveModelId: ownId, source: "org" };
      const inherited = platform(role);
      if (inherited) return { role, modelId: null, effectiveModelId: inherited, source: "platform" };
      const fallback = role === "decision" ? effective("chat.fast") : null;
      return fallback
        ? { role, modelId: null, effectiveModelId: fallback, source: "fallback" }
        : { role, modelId: null, effectiveModelId: null, source: "none" };
    }),
  );
}
