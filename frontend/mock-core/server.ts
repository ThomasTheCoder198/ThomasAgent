import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { createAgent, deleteAgent, getAgent, listAgents, patchAgent, resetAgents } from "./agents.ts";
import { guard, login, logout, me } from "./auth.ts";
import { resetCatalog, roles } from "./catalog.ts";
import { chat, resetRuns, stopRun } from "./chat/handler.ts";
import { mockConfig } from "./config.ts";
import { getObjectResponse, postObject, preflight } from "./fake-s3.ts";
import { getConversation, listConversations, resetConversations } from "./conversations.ts";
import { documentPages } from "./fixtures/refund-policy.ts";
import { sendData, sendError } from "./http.ts";
import { ragxProxy, resetKnowledgeBases } from "./knowledge-bases.ts";
import { createOrg, getOrg, listOrgs, patchOrg, provisionOrg, resetOrgs } from "./orgs.ts";
import {
  createProvider,
  deleteProvider,
  deleteRole,
  listModels,
  listOrgRoles,
  listProviders,
  patchModel,
  patchProvider,
  putRole,
  syncProvider,
  testModel,
} from "./registry.ts";

type Handler = (req: IncomingMessage, res: ServerResponse, params: string[]) => unknown;
type Route = { method: string; pattern: RegExp; handler: Handler; public?: boolean };

const PROXY_METHODS = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"] as const;
const RAGX_PROXY_PATTERN = /^\/api\/v1\/orgs\/([^/]+)\/apps\/ragx\/(.+)$/;
const ORG = "/api/v1/orgs/([^/]+)";
const orgOf = (raw: string | undefined) => decodeURIComponent(raw ?? "");
const idOf = (raw: string | undefined) => decodeURIComponent(raw ?? "");

const routes: Route[] = [
  { method: "POST", pattern: /^\/api\/v1\/auth\/login$/, handler: login, public: true },
  // Fake-core only, outside /api/v1 so the web app can never reach it: e2e restores seeded state per test.
  {
    method: "POST",
    pattern: /^\/__mock\/reset$/,
    handler: (req, res) => {
      const params = new URL(req.url ?? "/", "http://mock-core").searchParams;
      const agent = params.get("agent") ?? undefined;
      // Scoped resets (one seeded Agent by name; the model catalog) leave everything else alone: other specs
      // run in parallel against the same fake, and the full reset does not touch the catalog for the same reason.
      if (params.has("catalog")) resetCatalog();
      else if (agent) resetAgents(agent);
      else {
        resetOrgs();
        resetKnowledgeBases();
        resetAgents();
        resetConversations();
        resetRuns();
      }
      sendData(res, { status: "reset" });
    },
    public: true,
  },
  // The fake S3 is public and outside /api/v1, like MinIO: the browser posts and fetches bytes here directly.
  { method: "OPTIONS", pattern: /^\/__mock\/s3\/kb-raw(?:\/.*)?$/, handler: preflight, public: true },
  { method: "POST", pattern: /^\/__mock\/s3\/kb-raw$/, handler: postObject, public: true },
  {
    method: "GET",
    pattern: /^\/__mock\/s3\/kb-raw\/(.+)$/,
    handler: (req, res, [key]) => getObjectResponse(req, res, key ?? ""),
    public: true,
  },
  { method: "GET", pattern: /^\/api\/v1\/auth\/me$/, handler: me },
  { method: "POST", pattern: /^\/api\/v1\/auth\/logout$/, handler: logout },
  { method: "GET", pattern: /^\/api\/v1\/models$/, handler: listModels },
  {
    method: "PATCH",
    pattern: /^\/api\/v1\/models\/([^/]+)$/,
    handler: (q, r, [id]) => patchModel(q, r, idOf(id)),
  },
  {
    method: "POST",
    pattern: /^\/api\/v1\/models\/([^/]+)\/test$/,
    handler: (q, r, [id]) => testModel({ req: q, res: r }, idOf(id)),
  },
  { method: "GET", pattern: /^\/api\/v1\/providers$/, handler: listProviders },
  { method: "POST", pattern: /^\/api\/v1\/providers$/, handler: createProvider },
  {
    method: "PATCH",
    pattern: /^\/api\/v1\/providers\/([^/]+)$/,
    handler: (q, r, [id]) => patchProvider(q, r, idOf(id)),
  },
  {
    method: "DELETE",
    pattern: /^\/api\/v1\/providers\/([^/]+)$/,
    handler: (q, r, [id]) => deleteProvider({ req: q, res: r }, idOf(id)),
  },
  {
    method: "POST",
    pattern: /^\/api\/v1\/providers\/([^/]+)\/sync$/,
    handler: (q, r, [id]) => syncProvider({ req: q, res: r }, idOf(id)),
  },
  { method: "GET", pattern: /^\/api\/v1\/model-roles$/, handler: (_q, res) => sendData(res, roles) },
  {
    method: "PUT",
    pattern: /^\/api\/v1\/model-roles\/([^/]+)$/,
    handler: (q, r, [role]) => putRole(q, r, idOf(role)),
  },
  {
    method: "DELETE",
    pattern: /^\/api\/v1\/model-roles\/([^/]+)$/,
    handler: (q, r, [role]) => deleteRole({ req: q, res: r }, idOf(role)),
  },
  { method: "GET", pattern: /^\/api\/v1\/orgs$/, handler: listOrgs },
  { method: "POST", pattern: /^\/api\/v1\/orgs$/, handler: createOrg },
  {
    method: "GET",
    pattern: /^\/api\/v1\/orgs\/([^/]+)$/,
    handler: (req, res, [id]) => getOrg(req, res, decodeURIComponent(id ?? "")),
  },
  {
    method: "PATCH",
    pattern: /^\/api\/v1\/orgs\/([^/]+)$/,
    handler: (req, res, [id]) => patchOrg(req, res, decodeURIComponent(id ?? "")),
  },
  {
    method: "POST",
    pattern: /^\/api\/v1\/orgs\/([^/]+)\/provision$/,
    handler: (req, res, [id]) => provisionOrg(req, res, decodeURIComponent(id ?? "")),
  },
  // Core's generic app proxy to ragx: every method, one pattern, dispatched like ragx would.
  ...PROXY_METHODS.map((method): Route => ({ method, pattern: RAGX_PROXY_PATTERN, handler: ragxProxy })),
  {
    method: "GET",
    pattern: new RegExp(`^${ORG}/agents$`),
    handler: (q, r, [o]) => listAgents(q, r, orgOf(o)),
  },
  {
    method: "POST",
    pattern: new RegExp(`^${ORG}/agents$`),
    handler: (q, r, [o]) => createAgent(q, r, orgOf(o)),
  },
  {
    method: "GET",
    pattern: new RegExp(`^${ORG}/agents/([^/]+)$`),
    handler: (q, r, [o, id]) => getAgent(q, r, orgOf(o), idOf(id)),
  },
  {
    method: "PATCH",
    pattern: new RegExp(`^${ORG}/agents/([^/]+)$`),
    handler: (q, r, [o, id]) => patchAgent(q, r, orgOf(o), idOf(id)),
  },
  {
    method: "DELETE",
    pattern: new RegExp(`^${ORG}/agents/([^/]+)$`),
    handler: (q, r, [o, id]) => deleteAgent(q, r, orgOf(o), idOf(id)),
  },
  {
    method: "GET",
    pattern: new RegExp(`^${ORG}/conversations$`),
    handler: (_q, r, [o]) => sendData(r, listConversations(orgOf(o))),
  },
  {
    method: "GET",
    pattern: new RegExp(`^${ORG}/conversations/([^/]+)$`),
    handler: (q, r, [o, id]) => {
      const found = getConversation(orgOf(o), idOf(id));
      return found ? sendData(r, found) : sendError(q, r, ErrorCode.CONVERSATION_NOT_FOUND);
    },
  },
  { method: "POST", pattern: new RegExp(`^${ORG}/chat$`), handler: (q, r, [o]) => chat(q, r, orgOf(o)) },
  {
    method: "POST",
    pattern: new RegExp(`^${ORG}/runs/([^/]+)/stop$`),
    handler: (q, r, [o, id]) => stopRun(q, r, orgOf(o), idOf(id)),
  },
  {
    method: "GET",
    pattern: new RegExp(`^${ORG}/model-roles$`),
    handler: (q, r, [o]) => listOrgRoles({ req: q, res: r }, orgOf(o)),
  },
  {
    method: "PUT",
    pattern: new RegExp(`^${ORG}/model-roles/([^/]+)$`),
    handler: (q, r, [o, role]) => putRole(q, r, idOf(role), orgOf(o)),
  },
  {
    method: "DELETE",
    pattern: new RegExp(`^${ORG}/model-roles/([^/]+)$`),
    handler: (q, r, [o, role]) => deleteRole({ req: q, res: r }, idOf(role), orgOf(o)),
  },
  {
    method: "GET",
    pattern: /^\/api\/v1\/documents\/([^/]+)\/pages\/(\d+)$/,
    handler: (req, res, [documentId, page]) => {
      const found = documentPages.find((p) => p.documentId === documentId && String(p.page) === page);
      return found ? sendData(res, found) : sendError(req, res, ErrorCode.NOT_FOUND);
    },
  },
];

async function dispatch(req: IncomingMessage, res: ServerResponse) {
  const path = new URL(req.url ?? "/", "http://mock-core").pathname;
  for (const route of routes) {
    const match = route.pattern.exec(path);
    if (!match || route.method !== req.method) continue;
    if (!route.public && !guard(req, res)) return;
    await route.handler(req, res, match.slice(1));
    return;
  }
  sendError(req, res, ErrorCode.NOT_FOUND);
}

createServer((req, res) => {
  dispatch(req, res).catch((err: unknown) => {
    console.error("mock-core: unhandled", err);
    if (!res.headersSent) sendError(req, res, ErrorCode.INTERNAL_ERROR);
  });
}).listen(mockConfig.port, () => {
  console.log(`mock-core listening on http://localhost:${mockConfig.port} (owner: ${mockConfig.ownerEmail})`);
});
