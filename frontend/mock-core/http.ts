import { randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import { errorDefinitions, type ErrorCode } from "../lib/errors/codes.gen.ts";

export type Locale = "vi" | "en";

export const HTTP_OK = 200;

export function requestLocale(req: IncomingMessage): Locale {
  return req.headers["accept-language"]?.startsWith("en") ? "en" : "vi";
}

function writeJson(
  res: ServerResponse,
  status: number,
  body: unknown,
  headers: Record<string, string | string[]> = {},
) {
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", ...headers });
  res.end(JSON.stringify(body));
}

/** Success envelope, identical to core's `{ data, meta: { requestId } }`. */
export function sendData(
  res: ServerResponse,
  data: unknown,
  status = HTTP_OK,
  headers?: Record<string, string | string[]>,
) {
  writeJson(res, status, { data, meta: { requestId: `req_${randomUUID()}` } }, headers);
}

/** Error envelope with the catalog message in the caller's language. */
export function sendError(
  req: IncomingMessage,
  res: ServerResponse,
  code: ErrorCode,
  headers?: Record<string, string | string[]>,
) {
  const spec = errorDefinitions[code];
  const traceId = randomUUID().replaceAll("-", "");
  writeJson(
    res,
    spec.httpStatus,
    { error: { code, message: spec.message[requestLocale(req)], traceId } },
    headers,
  );
}

export async function readJson<T>(req: IncomingMessage): Promise<T | undefined> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  if (chunks.length === 0) return undefined;
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8")) as T;
  } catch {
    return undefined;
  }
}

export function readCookies(req: IncomingMessage): Map<string, string> {
  const jar = new Map<string, string>();
  for (const pair of (req.headers.cookie ?? "").split(";")) {
    const index = pair.indexOf("=");
    if (index > 0) jar.set(pair.slice(0, index).trim(), decodeURIComponent(pair.slice(index + 1).trim()));
  }
  return jar;
}

type CookieOptions = { maxAge: number; httpOnly: boolean };

export function cookie(name: string, value: string, { maxAge, httpOnly }: CookieOptions): string {
  const flags = [`Path=/`, `Max-Age=${maxAge}`, "SameSite=Lax", httpOnly ? "HttpOnly" : ""].filter(Boolean);
  return `${name}=${encodeURIComponent(value)}; ${flags.join("; ")}`;
}
