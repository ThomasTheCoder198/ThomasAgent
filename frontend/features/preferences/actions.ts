"use server";

import { cookies } from "next/headers";

import { isLocale, LOCALE_COOKIE, type Locale } from "@/i18n/config";

const ONE_YEAR_SECONDS = 31_536_000;

export async function setLocale(locale: Locale): Promise<void> {
  if (!isLocale(locale)) return;
  (await cookies()).set(LOCALE_COOKIE, locale, { path: "/", maxAge: ONE_YEAR_SECONDS, sameSite: "lax" });
}
