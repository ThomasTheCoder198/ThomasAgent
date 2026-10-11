import { randomUUID } from "node:crypto";
import type { IncomingMessage, ServerResponse } from "node:http";

import { REQUEST_ID_PATTERN } from "../lib/api/request-id.ts";
import { ErrorCode as Codes, errorDefinitions, type ErrorCode } from "../lib/errors/codes.gen.ts";

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
  const sent = req.headers["x-request-id"];
  const requestId = typeof sent === "string" && REQUEST_ID_PATTERN.test(sent) ? sent : `req_${randomUUID()}`;
  writeJson(
    res,
    spec.httpStatus,
    { error: { code, message: spec.message[requestLocale(req)], requestId } },
    { "X-Request-Id": requestId, ...headers },
  );
}

/** An error envelope that also carries `details.fields`, like core's validation and conflict answers. */
export function sendFieldError(
  req: IncomingMessage,
  res: ServerResponse,
  code: ErrorCode,
  fields: { field: string; code: string }[],
) {
  const spec = errorDefinitions[code];
  const sent = req.headers["x-request-id"];
  const requestId = typeof sent === "string" && REQUEST_ID_PATTERN.test(sent) ? sent : `req_${randomUUID()}`;
  writeJson(
    res,
    spec.httpStatus,
    { error: { code, message: spec.message[requestLocale(req)], details: { fields }, requestId } },
    { "X-Request-Id": requestId },
  );
}

export const HTTP_NO_CONTENT = 204;
const HTTP_UNAVAILABLE = 503;

/** Core's answer when ragx is down: HTTP 503, but the body shows only INTERNAL_ERROR + requestId (rule 4, F17). */
export function sendUnavailable(req: IncomingMessage, res: ServerResponse) {
  const spec = errorDefinitions[Codes.INTERNAL_ERROR];
  const sent = req.headers["x-request-id"];
  const requestId = typeof sent === "string" && REQUEST_ID_PATTERN.test(sent) ? sent : `req_${randomUUID()}`;
  writeJson(
    res,
    HTTP_UNAVAILABLE,
    { error: { code: Codes.INTERNAL_ERROR, message: spec.message[requestLocale(req)], requestId } },
    { "X-Request-Id": requestId },
  );
}

/** Ragx deletes answer 204 and core's app proxy passes it through. */
export function sendNoContent(res: ServerResponse) {
  res.writeHead(HTTP_NO_CONTENT);
  res.end();
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
