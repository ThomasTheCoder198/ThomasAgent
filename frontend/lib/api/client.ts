import { defaultLocale, type Locale } from "@/i18n/config";
import { ErrorCode, errorDefinitions } from "@/lib/errors/codes.gen";

export const CSRF_COOKIE = "thomas_csrf";
export const CSRF_HEADER = "X-CSRF-Token";
const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

type ErrorBody = { code: string; message: string; details?: Record<string, unknown>; traceId?: string };

export class ApiError extends Error {
  constructor(
    readonly code: ErrorCode,
    message: string,
    readonly status: number,
    readonly details?: Record<string, unknown>,
    readonly traceId?: string,
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

function networkError(locale: Locale): ApiError {
  const spec = errorDefinitions[ErrorCode.CLIENT_NETWORK_ERROR];
  return new ApiError(ErrorCode.CLIENT_NETWORK_ERROR, spec.message[locale], spec.httpStatus);
}

function isErrorCode(code: string): code is ErrorCode {
  return code in errorDefinitions;
}

/** Headers every state-changing call to core needs; also used by the chat stream transport. */
export function mutationHeaders(locale: Locale): Record<string, string> {
  const headers: Record<string, string> = { "Accept-Language": locale, "Content-Type": "application/json" };
  const csrf = readCookie(CSRF_COOKIE);
  if (csrf) headers[CSRF_HEADER] = decodeURIComponent(csrf);
  return headers;
}

function buildHeaders(init: RequestInit | undefined, locale: Locale): Headers {
  const headers = new Headers(init?.headers);
  headers.set("Accept-Language", locale);
  const method = (init?.method ?? "GET").toUpperCase();
  if (!SAFE_METHODS.has(method)) {
    for (const [name, value] of Object.entries(mutationHeaders(locale))) headers.set(name, value);
  }
  return headers;
}

export async function parseResponse<T>(res: Response, locale: Locale): Promise<T> {
  let body: { data?: T; error?: ErrorBody };
  try {
    body = (await res.json()) as typeof body;
  } catch {
    throw networkError(locale);
  }
  if (res.ok && body.data !== undefined) return body.data;
  const e = body.error;
  if (!e || !isErrorCode(e.code)) throw networkError(locale);
  throw new ApiError(e.code, e.message, res.status, e.details, e.traceId);
}

export async function apiFetch<T>(path: string, init?: RequestInit & { locale?: Locale }): Promise<T> {
  const locale = init?.locale ?? defaultLocale;
  let res: Response;
  try {
    res = await fetch(path, { ...init, credentials: "same-origin", headers: buildHeaders(init, locale) });
  } catch {
    throw networkError(locale);
  }
  return parseResponse<T>(res, locale);
}
