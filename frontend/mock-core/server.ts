import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

import { CHAT_PATHS } from "../features/chat/contract.ts";
import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { getAgent, listAgents, listKnowledgeBases, patchAgent, resetAgents } from "./agents.ts";
import { guard, login, logout, me } from "./auth.ts";
import { models, roles } from "./catalog.ts";
import { chat } from "./chat/handler.ts";
import { mockConfig } from "./config.ts";
import { defaultScope, getConversation, listConversations, seedConversations } from "./conversations.ts";
import { documentPages } from "./fixtures/refund-policy.ts";
import { sendData, sendError } from "./http.ts";

type Handler = (req: IncomingMessage, res: ServerResponse, params: string[]) => unknown;
type Route = { method: string; pattern: RegExp; handler: Handler; public?: boolean };

const routes: Route[] = [
  { method: "POST", pattern: /^\/api\/v1\/auth\/login$/, handler: login, public: true },
  // Fake-core only, outside /api/v1 so the web app can never reach it: e2e restores seeded state per test.
  {
    method: "POST",
    pattern: /^\/__mock\/reset$/,
    handler: (req, res) => {
      resetAgents(new URL(req.url ?? "/", "http://mock-core").searchParams.get("agent") ?? undefined);
      sendData(res, { status: "reset" });
    },
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
