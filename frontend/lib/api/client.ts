import { defaultLocale, type Locale } from "@/i18n/config";
import { ErrorCode, errorDefinitions } from "@/lib/errors/codes.gen";

import type { FilePage } from "@/lib/ragx/types";

import { REQUEST_ID_HEADER, newRequestId } from "./request-id";

export const CSRF_COOKIE = "thomas_csrf";
export const CSRF_HEADER = "X-CSRF-Token";
const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);
const NO_CONTENT = 204;

/** A list endpoint's `meta.page`: the cursor of the next page (null on the last) and the page size. */
export type Page = FilePage;

type ErrorBody = { code: string; message: string; details?: Record<string, unknown>; requestId?: string };
type Envelope<T> = { data?: T; error?: ErrorBody; meta?: { page?: Page } };

export class ApiError extends Error {
  constructor(
    readonly code: ErrorCode,
    message: string,
    readonly status: number,
    readonly details?: Record<string, unknown>,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  return document.cookie
    .split("; ")
    .find((c) => c.startsWith(`${name}=`))
    ?.slice(name.length + 1);
}

export function networkError(locale: Locale): ApiError {
  const spec = errorDefinitions[ErrorCode.CLIENT_NETWORK_ERROR];
  return new ApiError(ErrorCode.CLIENT_NETWORK_ERROR, spec.message[locale], spec.httpStatus);
}

function isErrorCode(code: string): code is ErrorCode {
  return code in errorDefinitions;
}

/** The CSRF header for a state-changing call, from the double-submit cookie; empty when there is no cookie. */
export function csrfHeaders(): Record<string, string> {
  const csrf = readCookie(CSRF_COOKIE);
  return csrf ? { [CSRF_HEADER]: decodeURIComponent(csrf) } : {};
}

/** Headers every state-changing call to core needs (including a fresh request id); also used by the chat transport. */
export function mutationHeaders(locale: Locale): Record<string, string> {
  return {
    "Accept-Language": locale,
    "Content-Type": "application/json",
    [REQUEST_ID_HEADER]: newRequestId(),
    ...csrfHeaders(),
  };
}

function buildHeaders(init: RequestInit | undefined, locale: Locale): Headers {
  const headers = new Headers(init?.headers);
  headers.set("Accept-Language", locale);
  const method = (init?.method ?? "GET").toUpperCase();
  if (!SAFE_METHODS.has(method)) {
    for (const [name, value] of Object.entries(mutationHeaders(locale))) headers.set(name, value);
  } else {
    headers.set(REQUEST_ID_HEADER, newRequestId());
  }
  return headers;
}

export async function parseEnvelope<T>(res: Response, locale: Locale): Promise<{ data: T; page?: Page }> {
  // Ragx deletes answer 204 and core's app proxy passes it through unchanged.
  if (res.status === NO_CONTENT) return { data: undefined as T };
  let body: Envelope<T>;
  try {
    body = (await res.json()) as Envelope<T>;
  } catch {
    throw networkError(locale);
  }
  if (res.ok && body.data !== undefined) return { data: body.data, page: body.meta?.page };
  const e = body.error;
  if (!e || !isErrorCode(e.code)) throw networkError(locale);
  throw new ApiError(
    e.code,
    e.message,
    res.status,
    e.details,
    e.requestId ?? res.headers.get(REQUEST_ID_HEADER) ?? undefined,
  );
}

export async function parseResponse<T>(res: Response, locale: Locale): Promise<T> {
  return (await parseEnvelope<T>(res, locale)).data;
}

type FetchInit = RequestInit & { locale?: Locale };

async function send(path: string, init: FetchInit | undefined, locale: Locale): Promise<Response> {
  try {
    return await fetch(path, { ...init, credentials: "same-origin", headers: buildHeaders(init, locale) });
  } catch {
    throw networkError(locale);
  }
}

export async function apiFetch<T>(path: string, init?: FetchInit): Promise<T> {
  const locale = init?.locale ?? defaultLocale;
  return parseResponse<T>(await send(path, init, locale), locale);
}

/** Like apiFetch, for list endpoints whose `meta.page` carries the next cursor (main spec §18.2). */
export async function apiFetchPage<T>(path: string, init?: FetchInit): Promise<{ data: T; page: Page }> {
  const locale = init?.locale ?? defaultLocale;
  const { data, page } = await parseEnvelope<T>(await send(path, init, locale), locale);
  return { data, page: page ?? { cursor: null, limit: 0 } };
}

/** The message for a failed call; appends the request id so the user can quote it to the operator. */
export function describeApiError(
  err: unknown,
  fallback: string,
  formatRequestId: (id: string) => string,
): string {
  if (!(err instanceof ApiError)) return fallback;
  return err.requestId ? `${err.message} ${formatRequestId(err.requestId)}` : err.message;
}
