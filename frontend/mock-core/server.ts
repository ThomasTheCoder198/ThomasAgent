import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

import { CHAT_PATHS } from "../features/chat/contract.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { getAgent, listAgents, listKnowledgeBases, patchAgent, resetAgents } from "./agents.ts";
import { guard, login, logout, me } from "./auth.ts";
import { models, roles } from "./catalog.ts";
import { chat } from "./chat/handler.ts";
import { mockConfig } from "./config.ts";
import { getObjectResponse, postObject, preflight } from "./fake-s3.ts";
import { defaultScope, getConversation, listConversations, seedConversations } from "./conversations.ts";
import { documentPages } from "./fixtures/refund-policy.ts";
import { sendData, sendError } from "./http.ts";
import { ragxProxy, resetKnowledgeBases } from "./knowledge-bases.ts";
import { createOrg, getOrg, listOrgs, patchOrg, provisionOrg, resetOrgs } from "./orgs.ts";

type Handler = (req: IncomingMessage, res: ServerResponse, params: string[]) => unknown;
type Route = { method: string; pattern: RegExp; handler: Handler; public?: boolean };

const PROXY_METHODS = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"] as const;
const RAGX_PROXY_PATTERN = /^\/api\/v1\/orgs\/([^/]+)\/apps\/ragx\/(.+)$/;

const routes: Route[] = [
  { method: "POST", pattern: /^\/api\/v1\/auth\/login$/, handler: login, public: true },
  // Fake-core only, outside /api/v1 so the web app can never reach it: e2e restores seeded state per test.
  {
    method: "POST",
    pattern: /^\/__mock\/reset$/,
    handler: (req, res) => {
      const agent = new URL(req.url ?? "/", "http://mock-core").searchParams.get("agent") ?? undefined;
      // A reset scoped to one Agent (the Agents e2e, per test) leaves orgs and KBs alone: other specs run in parallel.
      if (!agent) {
        resetOrgs();
        resetKnowledgeBases();
      }
      resetAgents(agent);
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
  { method: "GET", pattern: /^\/api\/v1\/models$/, handler: (_q, res) => sendData(res, models) },
  { method: "GET", pattern: /^\/api\/v1\/model-roles$/, handler: (_q, res) => sendData(res, roles) },
  { method: "GET", pattern: /^\/api\/v1\/chat\/scope$/, handler: (_q, res) => sendData(res, defaultScope) },
  { method: "GET", pattern: /^\/api\/v1\/agents$/, handler: listAgents },
  {
    method: "GET",
    pattern: /^\/api\/v1\/agents\/([^/]+)$/,
    handler: (req, res, [id]) => getAgent(req, res, id ?? ""),
  },
  {
    method: "PATCH",
    pattern: /^\/api\/v1\/agents\/([^/]+)$/,
    handler: (req, res, [id]) => patchAgent(req, res, id ?? ""),
  },
  { method: "GET", pattern: /^\/api\/v1\/knowledge-bases$/, handler: listKnowledgeBases },
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
  { method: "POST", pattern: new RegExp(`^${CHAT_PATHS.chat}$`), handler: chat },
  {
    method: "GET",
    pattern: /^\/api\/v1\/conversations$/,
    handler: (_q, res) => sendData(res, listConversations()),
  },
  {
    method: "GET",
    pattern: /^\/api\/v1\/conversations\/([^/]+)$/,
    handler: (req, res, [id]) => {
      const found = getConversation(decodeURIComponent(id ?? ""));
      return found ? sendData(res, found) : sendError(req, res, ErrorCode.NOT_FOUND);
    },
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

await seedConversations();

createServer((req, res) => {
  dispatch(req, res).catch((err: unknown) => {
    console.error("mock-core: unhandled", err);
    if (!res.headersSent) sendError(req, res, ErrorCode.INTERNAL_ERROR);
  });
}).listen(mockConfig.port, () => {
  console.log(`mock-core listening on http://localhost:${mockConfig.port} (owner: ${mockConfig.ownerEmail})`);
});
