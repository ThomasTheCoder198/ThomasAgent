import "server-only";

import { cookies, headers } from "next/headers";

import { type Locale } from "@/i18n/config";
import { env } from "@/lib/env";

import { ErrorCode } from "@/lib/errors/codes.gen";

import { ApiError, parseResponse } from "./client";
import type { Loaded } from "./loaded";

async function requestLocale(): Promise<Locale> {
  return (await headers()).get("accept-language")?.startsWith("en") ? "en" : "vi";
}

/** Server components call core directly and forward the browser's cookies, so the session stays first-party. */
export async function serverFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const cookieHeader = (await cookies()).toString();
  const lang = await requestLocale();
  const res = await fetch(`${env.WEB_CORE_URL}${path}`, {
    ...init,
    cache: "no-store",
    headers: { ...init?.headers, Cookie: cookieHeader, "Accept-Language": lang },
  });
  return parseResponse<T>(res, lang);
}

export type { Loaded } from "./loaded";

/**
 * For data a page needs but can live without: failures (404 or 5xx) come back as a value the page renders as
 * an error state, instead of an empty list that would look like "nothing configured".
 */
export async function serverFetchResult<T>(path: string): Promise<Loaded<T>> {
  try {
    return { ok: true, data: await serverFetch<T>(path) };
  } catch (err) {
    if (err instanceof ApiError) return { ok: false, message: err.message };
    throw err;
  }
}

/** For endpoints core may not serve yet (provisional M2 routes): a 404 means "nothing there", not a failure. */
export async function serverFetchOptional<T>(path: string, fallback: T): Promise<T> {
  try {
    return await serverFetch<T>(path);
  } catch (err) {
    if (err instanceof ApiError && err.code === ErrorCode.NOT_FOUND) return fallback;
    throw err;
  }
}
