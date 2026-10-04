import { randomBytes, randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import { ErrorCode } from "../lib/errors/codes.gen.ts";
import { mockConfig } from "./config.ts";
import { cookie, HTTP_OK, readCookies, readJson, sendData, sendError } from "./http.ts";

export const SESSION_COOKIE = "thomas_session";
export const CSRF_COOKIE = "thomas_csrf";
const CSRF_HEADER = "x-csrf-token";
const TOKEN_BYTES = 32;
const MS_PER_SECOND = 1000;

type Session = { csrf: string; expiresAt: number };

const owner = { id: randomUUID(), email: mockConfig.ownerEmail, displayName: mockConfig.ownerName };
const sessions = new Map<string, Session>();

function currentSession(req: IncomingMessage): Session | undefined {
  const token = readCookies(req).get(SESSION_COOKIE);
  const session = token ? sessions.get(token) : undefined;
  if (session && session.expiresAt > Date.now()) return session;
  return undefined;
}

/** Mirrors core: every route except login needs a live session; unsafe methods also need the CSRF header. */
export function guard(req: IncomingMessage, res: ServerResponse): boolean {
  const session = currentSession(req);
  if (!session) {
    sendError(req, res, ErrorCode.UNAUTHENTICATED);
    return false;
  }
  const unsafe = !["GET", "HEAD", "OPTIONS"].includes(req.method ?? "GET");
  if (unsafe && req.headers[CSRF_HEADER] !== session.csrf) {
    sendError(req, res, ErrorCode.AUTH_CSRF_INVALID);
    return false;
  }
  return true;
}

export async function login(req: IncomingMessage, res: ServerResponse) {
  const body = await readJson<{ email?: string; password?: string }>(req);
  if (body?.email !== mockConfig.ownerEmail || body.password !== mockConfig.ownerPassword) {
    sendError(req, res, ErrorCode.AUTH_INVALID_CREDENTIALS);
    return;
  }
  const token = randomBytes(TOKEN_BYTES).toString("base64url");
  const csrf = randomBytes(TOKEN_BYTES).toString("base64url");
  const expiresAt = Date.now() + mockConfig.sessionTtlSeconds * MS_PER_SECOND;
  sessions.set(token, { csrf, expiresAt });
  const maxAge = mockConfig.sessionTtlSeconds;
  sendData(res, { user: owner, csrfToken: csrf, expiresAt: new Date(expiresAt).toISOString() }, HTTP_OK, {
    "Set-Cookie": [
      cookie(SESSION_COOKIE, token, { maxAge, httpOnly: true }),
      cookie(CSRF_COOKIE, csrf, { maxAge, httpOnly: false }),
    ],
  });
}

export function me(_req: IncomingMessage, res: ServerResponse) {
  sendData(res, { user: owner });
}

export function logout(req: IncomingMessage, res: ServerResponse) {
  const token = readCookies(req).get(SESSION_COOKIE);
  if (token) sessions.delete(token);
  sendData(res, { status: "ok" }, HTTP_OK, {
    "Set-Cookie": [
      cookie(SESSION_COOKIE, "", { maxAge: 0, httpOnly: true }),
      cookie(CSRF_COOKIE, "", { maxAge: 0, httpOnly: false }),
    ],
  });
}
