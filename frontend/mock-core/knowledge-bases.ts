import { randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import type { Organization } from "../features/orgs/contract.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import type { KbFile, KnowledgeBase, UploadTicket } from "../lib/ragx/types.ts";
import { deleteObject, getObject, putObject, resetObjects } from "./fake-s3.ts";
import { mockConfig } from "./config.ts";
import {
  HTTP_OK,
  readJson,
  sendData,
  sendError,
  sendFieldError,
  sendNoContent,
  sendUnavailable,
} from "./http.ts";
import { DEMO_ORG_ID, DOWN_SUFFIX, findOrg } from "./orgs.ts";

/**
 * Ragx's KB API as core's app proxy exposes it: /api/v1/orgs/{orgId}/apps/ragx/<ragx path without /v1>.
 * Handlers are named `ragx...` (F26) so they cannot be mistaken for core's own routes.
 */
const HTTP_CREATED = 201;
const DEFAULT_LIMIT = 20;
const MAX_LIMIT = 100;
const MAX_UPLOAD_BYTES = 52_428_800;
const UPLOAD_TTL_MS = 900_000;
const DOWNLOAD_TTL_MS = 60_000;
const MOCK_SIGNATURE = "mock";
const BUCKET_URL_PATH = "/__mock/s3/kb-raw";
const EMBEDDING_MODEL = "text-embedding-3-small";
const EMBEDDING_DIMS = 1536;
const SEEDED_AT = "2026-10-10T08:00:00Z";
const POLICY_KEY_FIELD = "$key";
const OCTET_TYPE = "application/octet-stream";
/** Each list call moves an in-progress file one step, so polling shows it walking to ready. */
const NEXT_STATUS: Partial<Record<KbFile["status"], KbFile["status"]>> = {
  pending: "parsing",
  parsing: "indexing",
  indexing: "ready",
};

type OrgData = { kbs: KnowledgeBase[]; files: KbFile[]; slots: Map<string, Slot> };
type Slot = { kbId: string; name: string; sizeBytes: number; key: string };
type Ctx = { req: IncomingMessage; res: ServerResponse; orgId: string; data: OrgData; params: string[] };

let orgs = new Map<string, OrgData>();

/** Joins `POST /__mock/reset`. */
export function resetKnowledgeBases() {
  orgs = new Map();
  resetObjects();
}

const objectKey = (orgId: string, kbId: string, fileId: string) => `${orgId}/${kbId}/${fileId}`;
const now = () => new Date().toISOString();

function makeKb(id: string, name: string, isDefault: boolean, at = now()): KnowledgeBase {
  return {
    id,
    name,
    isDefault,
    embeddingModel: EMBEDDING_MODEL,
    embeddingDims: EMBEDDING_DIMS,
    fileCount: 0,
    createdAt: at,
    updatedAt: at,
  };
}

function seedFile(
  orgId: string,
  kbId: string,
  name: string,
  status: KbFile["status"],
  errorCode = "",
): KbFile {
  const id = randomUUID();
  const stored = putObject(
    objectKey(orgId, kbId, id),
    Buffer.from(`# ${name}\n\nSynthetic fixture.\n`),
    name,
  );
  return {
    id,
    kbId,
    name,
    version: 1,
    contentType: "text/markdown",
    sizeBytes: stored.bytes.length,
    sha256: stored.sha256,
    status,
    errorCode,
    degraded: false,
    createdAt: SEEDED_AT,
    updatedAt: SEEDED_AT,
  };
}

/** Per org: its default KB (the id core stored), plus a second KB and three files for the demo org. */
function dataFor(orgId: string, org: Organization): OrgData {
  let data = orgs.get(orgId);
  if (data) return data;
  const defaultId = org.defaultKnowledgeBaseId ?? randomUUID();
  data = { kbs: [makeKb(defaultId, "Mặc định", true, org.createdAt)], files: [], slots: new Map() };
  if (orgId === DEMO_ORG_ID) {
    data.kbs.push(makeKb(randomUUID(), "Chính sách", false, org.createdAt));
    data.files = [
      seedFile(orgId, defaultId, "chinh-sach-hoan-tien.md", "ready"),
      seedFile(orgId, defaultId, "bang-gia-2026.md", "parsing"),
      seedFile(orgId, defaultId, "hop-dong-mau.md", "failed", "INGEST_PARSE_FAILED"),
    ].reverse();
  }
  orgs.set(orgId, data);
  return data;
}

const withCount = (data: OrgData, kb: KnowledgeBase): KnowledgeBase => ({
  ...kb,
  fileCount: data.files.filter((f) => f.kbId === kb.id).length,
});

function findKb(ctx: Ctx, kbId: string): KnowledgeBase | undefined {
  return ctx.data.kbs.find((k) => k.id === kbId);
}

function advance(data: OrgData, kbId: string) {
  data.files = data.files.map((file) => {
    const next = NEXT_STATUS[file.status];
    return file.kbId === kbId && next ? { ...file, status: next, updatedAt: now() } : file;
  });
}

function nameOf(body: { name?: unknown } | undefined): string | undefined {
  const name = typeof body?.name === "string" ? body.name.trim() : "";
  return name || undefined;
}

function sameName(data: OrgData, name: string, exceptId?: string) {
  return data.kbs.some((k) => k.id !== exceptId && k.name.toLowerCase() === name.toLowerCase());
}

async function ragxCreateKb({ req, res, data }: Ctx) {
  const name = nameOf(await readJson(req));
  if (!name) return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  if (sameName(data, name))
    return sendFieldError(req, res, ErrorCode.CONFLICT, [{ field: "name", code: "TAKEN" }]);
  const kb = makeKb(randomUUID(), name, false);
  data.kbs.push(kb);
  sendData(res, kb, HTTP_CREATED);
}

async function ragxRenameKb(ctx: Ctx) {
  const { req, res, data, params } = ctx;
  const kb = findKb(ctx, params[0] ?? "");
  if (!kb) return sendError(req, res, ErrorCode.KB_NOT_FOUND);
  const name = nameOf(await readJson(req));
  if (!name) return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  if (sameName(data, name, kb.id)) {
    return sendFieldError(req, res, ErrorCode.CONFLICT, [{ field: "name", code: "TAKEN" }]);
  }
  Object.assign(kb, { name, updatedAt: now() });
  sendData(res, withCount(data, kb));
}

function ragxDeleteKb(ctx: Ctx) {
  const { req, res, orgId, data } = ctx;
  const kb = findKb(ctx, ctx.params[0] ?? "");
  if (!kb) return sendError(req, res, ErrorCode.KB_NOT_FOUND);
  if (kb.isDefault) return sendError(req, res, ErrorCode.CONFLICT);
  for (const file of data.files.filter((f) => f.kbId === kb.id))
    deleteObject(objectKey(orgId, kb.id, file.id));
  data.kbs = data.kbs.filter((k) => k.id !== kb.id);
  data.files = data.files.filter((f) => f.kbId !== kb.id);
  sendNoContent(res);
}

function ragxListFiles(ctx: Ctx) {
  const { req, res, data } = ctx;
  const kb = findKb(ctx, ctx.params[0] ?? "");
  if (!kb) return sendError(req, res, ErrorCode.KB_NOT_FOUND);
  advance(data, kb.id);
  const query = new URL(req.url ?? "/", mockConfig.publicUrl).searchParams;
  const limit = Math.min(Number(query.get("limit")) || DEFAULT_LIMIT, MAX_LIMIT);
  const start = Number(Buffer.from(query.get("cursor") ?? "", "base64").toString("utf8")) || 0;
  const all = data.files.filter((f) => f.kbId === kb.id);
  const next = start + limit;
  const cursor = next < all.length ? Buffer.from(String(next)).toString("base64") : null;
  res.writeHead(HTTP_OK, { "Content-Type": "application/json; charset=utf-8" });
  res.end(
    JSON.stringify({
      data: all.slice(start, next),
      meta: { requestId: `req_${randomUUID()}`, page: { cursor, limit } },
    }),
  );
}

async function ragxPresign(ctx: Ctx) {
  const { req, res, orgId, data } = ctx;
  const kb = findKb(ctx, ctx.params[0] ?? "");
  if (!kb) return sendError(req, res, ErrorCode.KB_NOT_FOUND);
  const body = await readJson<{ name?: unknown; sizeBytes?: unknown }>(req);
  const size = body?.sizeBytes;
  if (typeof body?.name !== "string" || typeof size !== "number" || size < 1 || size > MAX_UPLOAD_BYTES) {
    return sendError(req, res, ErrorCode.VALIDATION_FAILED);
  }
  const fileId = randomUUID();
  const key = objectKey(orgId, kb.id, fileId);
  data.slots.set(fileId, { kbId: kb.id, name: body.name, sizeBytes: size, key });
  const policy = {
    conditions: [
      ["eq", POLICY_KEY_FIELD, key],
      ["content-length-range", size, size],
    ],
  };
  const ticket: UploadTicket = {
    fileId,
    upload: {
      url: `${mockConfig.publicUrl}${BUCKET_URL_PATH}`,
      fields: {
        key,
        policy: Buffer.from(JSON.stringify(policy)).toString("base64"),
        "x-amz-signature": MOCK_SIGNATURE,
      },
    },
    expiresAt: new Date(Date.now() + UPLOAD_TTL_MS).toISOString(),
  };
  sendData(res, ticket, HTTP_CREATED);
}

function versionOf(data: OrgData, kbId: string, name: string) {
  return data.files.filter((f) => f.kbId === kbId && f.name === name).length + 1;
}

/** Idempotent by file id; the same bytes as an existing file answer that file (and drop the new object). */
function ragxConfirm(ctx: Ctx) {
  const { req, res, data } = ctx;
  const [kbId = "", fileId = ""] = ctx.params;
  if (!findKb(ctx, kbId)) return sendError(req, res, ErrorCode.KB_NOT_FOUND);
  const existing = data.files.find((f) => f.id === fileId && f.kbId === kbId);
  if (existing) return sendData(res, existing);
  const slot = data.slots.get(fileId);
  if (!slot || slot.kbId !== kbId) return sendError(req, res, ErrorCode.KB_FILE_NOT_FOUND);
  const stored = getObject(slot.key);
  if (!stored)
    return sendFieldError(req, res, ErrorCode.VALIDATION_FAILED, [{ field: "file", code: "MISSING" }]);
  const duplicate = data.files.find((f) => f.kbId === kbId && f.sha256 === stored.sha256);
  data.slots.delete(fileId);
  if (duplicate) {
    deleteObject(slot.key);
    return sendData(res, duplicate);
  }
  const at = now();
  const file: KbFile = {
    id: fileId,
    kbId,
    name: slot.name,
    version: versionOf(data, kbId, slot.name),
    contentType: OCTET_TYPE,
    sizeBytes: stored.bytes.length,
    sha256: stored.sha256,
    status: "pending",
    errorCode: "",
    degraded: false,
    createdAt: at,
    updatedAt: at,
  };
  data.files = [file, ...data.files];
  sendData(res, file);
}

function visibleFile(ctx: Ctx): KbFile | undefined {
  const [kbId = "", fileId = ""] = ctx.params;
  return ctx.data.files.find((f) => f.id === fileId && f.kbId === kbId);
}

function ragxDownloadUrl(ctx: Ctx) {
  const { req, res, orgId } = ctx;
  const file = visibleFile(ctx);
  if (!file) return sendError(req, res, ErrorCode.KB_FILE_NOT_FOUND);
  const url = `${mockConfig.publicUrl}${BUCKET_URL_PATH}/${objectKey(orgId, file.kbId, file.id)}`;
  sendData(res, { url, expiresAt: new Date(Date.now() + DOWNLOAD_TTL_MS).toISOString() });
}

function ragxRetryFile(ctx: Ctx) {
  const { req, res, data } = ctx;
  const file = visibleFile(ctx);
  if (!file) return sendError(req, res, ErrorCode.KB_FILE_NOT_FOUND);
  if (file.status !== "failed") return sendError(req, res, ErrorCode.CONFLICT);
  const retried: KbFile = { ...file, status: "pending", errorCode: "", updatedAt: now() };
  data.files = data.files.map((f) => (f.id === file.id ? retried : f));
  sendData(res, retried);
}

function ragxDeleteFile(ctx: Ctx) {
  const { req, res, orgId, data } = ctx;
  const file = visibleFile(ctx);
  if (!file) return sendError(req, res, ErrorCode.KB_FILE_NOT_FOUND);
  deleteObject(objectKey(orgId, file.kbId, file.id));
  data.files = data.files.filter((f) => f.id !== file.id);
  sendNoContent(res);
}

type RagxRoute = { method: string; pattern: RegExp; handler: (ctx: Ctx) => unknown };

const KB = "knowledge-bases";
const ragxRoutes: RagxRoute[] = [
  {
    method: "GET",
    pattern: new RegExp(`^${KB}$`),
    handler: (c) =>
      sendData(
        c.res,
        c.data.kbs.map((k) => withCount(c.data, k)),
      ),
  },
  { method: "POST", pattern: new RegExp(`^${KB}$`), handler: ragxCreateKb },
  {
    method: "GET",
    pattern: new RegExp(`^${KB}/([^/]+)$`),
    handler: (c) => {
      const kb = findKb(c, c.params[0] ?? "");
      return kb ? sendData(c.res, withCount(c.data, kb)) : sendError(c.req, c.res, ErrorCode.KB_NOT_FOUND);
    },
  },
  { method: "PATCH", pattern: new RegExp(`^${KB}/([^/]+)$`), handler: ragxRenameKb },
  { method: "DELETE", pattern: new RegExp(`^${KB}/([^/]+)$`), handler: ragxDeleteKb },
  { method: "GET", pattern: new RegExp(`^${KB}/([^/]+)/files$`), handler: ragxListFiles },
  { method: "POST", pattern: new RegExp(`^${KB}/([^/]+)/uploads$`), handler: ragxPresign },
  { method: "POST", pattern: new RegExp(`^${KB}/([^/]+)/files/([^/]+)/confirm$`), handler: ragxConfirm },
  {
    method: "GET",
    pattern: new RegExp(`^${KB}/([^/]+)/files/([^/]+)/download-url$`),
    handler: ragxDownloadUrl,
  },
  { method: "POST", pattern: new RegExp(`^${KB}/([^/]+)/files/([^/]+)/retry$`), handler: ragxRetryFile },
  { method: "DELETE", pattern: new RegExp(`^${KB}/([^/]+)/files/([^/]+)$`), handler: ragxDeleteFile },
];

/** Core's app proxy: an unknown org is 404, an org ragx has not prepared is 409, anything else is dispatched like ragx would. */
export function ragxProxy(
  req: IncomingMessage,
  res: ServerResponse,
  [rawOrgId = "", rawRest = ""]: string[],
) {
  const orgId = decodeURIComponent(rawOrgId);
  const rest = decodeURIComponent(rawRest);
  const org = findOrg(orgId);
  if (!org) return sendError(req, res, ErrorCode.NOT_FOUND);
  if (org.ragxStatus !== "ready") return sendError(req, res, ErrorCode.ORG_NOT_PROVISIONED);
  // A rest starting with `orgs/` stands for ragx's provisioning route, which core's proxy answers as 404 (scope 403 -> 404).
  if (rest.startsWith("orgs/")) return sendError(req, res, ErrorCode.NOT_FOUND);
  if (org.name.endsWith(DOWN_SUFFIX)) return sendUnavailable(req, res);
  const data = dataFor(orgId, org);
  for (const route of ragxRoutes) {
    const match = route.pattern.exec(rest);
    if (match && route.method === req.method)
      return route.handler({ req, res, orgId, data, params: match.slice(1) });
  }
  sendError(req, res, ErrorCode.NOT_FOUND);
}
