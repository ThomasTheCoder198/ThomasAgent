import { randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import type { Organization, OrganizationInput } from "../features/orgs/contract.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { readJson, requestLocale, sendData, sendError } from "./http.ts";

const HTTP_CREATED = 201;
const HTTP_CONFLICT = 409;
/** A name with this suffix makes create answer 201 `failed`, so the failure path can be exercised by hand. */
const FAIL_SUFFIX = "!fail";
/** A name with this suffix makes create answer 201 `failed` with LLM_ROLE_NOT_ASSIGNED; Retry then succeeds. */
const ROLE_SUFFIX = "!role";
/** A name with this suffix makes the org's ragx proxy routes answer 503 INTERNAL_ERROR (ragx down), org itself stays ready. */
export const DOWN_SUFFIX = "!down";
const SEEDED_AT = "2026-10-10T08:00:00Z";

/** Fixed ids: Task 7's fixtures and e2e specs point at them. */
export const DEMO_ORG_ID = "3f2b7c1e-5a4d-4b8e-9c10-6d2f8a1b0e01";
const BETA_ORG_ID = "3f2b7c1e-5a4d-4b8e-9c10-6d2f8a1b0e02";
const GAMMA_ORG_ID = "3f2b7c1e-5a4d-4b8e-9c10-6d2f8a1b0e03";
const DEMO_KB_ID = "9a1c2d3e-4f50-4617-8899-aabbccddee01";

const seed = (over: Partial<Organization> & Pick<Organization, "id" | "name">): Organization => ({
  externalParsing: true,
  ragxStatus: "ready",
  ragxErrorCode: null,
  settingsSynced: true,
  defaultKnowledgeBaseId: null,
  createdAt: SEEDED_AT,
  updatedAt: SEEDED_AT,
  ...over,
});

const seeded = (): Organization[] => [
  seed({ id: DEMO_ORG_ID, name: "Công ty Demo", defaultKnowledgeBaseId: DEMO_KB_ID }),
  seed({
    id: BETA_ORG_ID,
    name: "Beta",
    ragxStatus: "failed",
    ragxErrorCode: "LLM_ROLE_NOT_ASSIGNED",
    settingsSynced: false,
  }),
  // A process died mid-call: provisioning is synchronous, so this org stays pending until Retry.
  seed({ id: GAMMA_ORG_ID, name: "Gamma", ragxStatus: "pending", settingsSynced: false }),
];

let store = seeded();

/** Joins `POST /__mock/reset`. */
export function resetOrgs() {
  store = seeded();
}

/** For the proxy routes of the Knowledge Base surface: the org the path names, or undefined. */
export function findOrg(id: string): Organization | undefined {
  return store.find((org) => org.id === id);
}

const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

function nameTaken(req: IncomingMessage, res: ServerResponse) {
  const spec = { field: "name", code: "TAKEN" };
  res.writeHead(HTTP_CONFLICT, { "Content-Type": "application/json; charset=utf-8" });
  res.end(
    JSON.stringify({
      error: {
        code: ErrorCode.CONFLICT,
        message: sendErrorMessage(req),
        details: { fields: [spec] },
        requestId: `req_${randomUUID()}`,
      },
    }),
  );
}

function sendErrorMessage(req: IncomingMessage): string {
  return requestLocale(req) === "en"
    ? "An organization with this name already exists."
    : "Đã có tổ chức trùng tên.";
}

export function listOrgs(_req: IncomingMessage, res: ServerResponse) {
  sendData(res, store);
}

export function getOrg(req: IncomingMessage, res: ServerResponse, id: string) {
  const org = findOrg(id);
  if (!org) return sendError(req, res, ErrorCode.NOT_FOUND);
  sendData(res, org);
}

function parseCreate(input: Partial<OrganizationInput> | undefined): OrganizationInput | null {
  const name = input?.name?.trim();
  return name && typeof input?.externalParsing === "boolean"
    ? { name, externalParsing: input.externalParsing }
    : null;
}

export async function createOrg(req: IncomingMessage, res: ServerResponse) {
  const input = parseCreate(await readJson<OrganizationInput>(req));
  if (!input) return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  const { name } = input;
  if (store.some((org) => sameName(org.name, name))) return nameTaken(req, res);
  const failing = name.endsWith(FAIL_SUFFIX) || name.endsWith(ROLE_SUFFIX);
  const failureCode = name.endsWith(ROLE_SUFFIX)
    ? ErrorCode.LLM_ROLE_NOT_ASSIGNED
    : ErrorCode.RAGX_UNAVAILABLE;
  const now = new Date().toISOString();
  const created: Organization = {
    id: randomUUID(),
    name,
    externalParsing: input.externalParsing,
    ragxStatus: failing ? "failed" : "ready",
    ragxErrorCode: failing ? failureCode : null,
    settingsSynced: !failing,
    defaultKnowledgeBaseId: failing ? null : randomUUID(),
    createdAt: now,
    updatedAt: now,
  };
  store = [...store, created];
  sendData(res, created, HTTP_CREATED);
}

/** Mirrors core's P2: a ready org stays synced; any other state only changes the row. */
export async function patchOrg(req: IncomingMessage, res: ServerResponse, id: string) {
  const org = findOrg(id);
  if (!org) return sendError(req, res, ErrorCode.NOT_FOUND);
  const patch = await readJson<Partial<OrganizationInput>>(req);
  if (!patch) return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  const name = patch.name?.trim();
  if (name !== undefined && (!name || store.some((o) => o.id !== id && sameName(o.name, name)))) {
    return name ? nameTaken(req, res) : sendError(req, res, ErrorCode.VALIDATION_FAILED);
  }
  const updated: Organization = {
    ...org,
    name: name ?? org.name,
    externalParsing: patch.externalParsing ?? org.externalParsing,
    updatedAt: new Date().toISOString(),
  };
  store = store.map((o) => (o.id === id ? updated : o));
  sendData(res, updated);
}

/** Synchronous provisioning: answers 200 with the real state, which in the fake is always ready. */
export function provisionOrg(req: IncomingMessage, res: ServerResponse, id: string) {
  const org = findOrg(id);
  if (!org) return sendError(req, res, ErrorCode.NOT_FOUND);
  const updated: Organization = {
    ...org,
    ragxStatus: "ready",
    ragxErrorCode: null,
    settingsSynced: true,
    defaultKnowledgeBaseId: org.defaultKnowledgeBaseId ?? randomUUID(),
    updatedAt: new Date().toISOString(),
  };
  store = store.map((o) => (o.id === id ? updated : o));
  sendData(res, updated);
}
