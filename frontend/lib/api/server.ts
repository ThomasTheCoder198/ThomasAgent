import "server-only";

import { cookies, headers } from "next/headers";

import { type Locale } from "@/i18n/config";
import { env } from "@/lib/env";

import { parseResponse } from "./client";

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
