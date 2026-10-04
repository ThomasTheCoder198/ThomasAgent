import { cookies, headers } from "next/headers";
import { getRequestConfig } from "next-intl/server";

import { defaultLocale, isLocale, LOCALE_COOKIE, type Locale } from "./config";

const LANGUAGE_PREFIX_LENGTH = 2;

export default getRequestConfig(async () => {
  const fromCookie = (await cookies()).get(LOCALE_COOKIE)?.value;
  const fromHeader = (await headers()).get("accept-language")?.slice(0, LANGUAGE_PREFIX_LENGTH);
  const locale: Locale = isLocale(fromCookie)
    ? fromCookie
    : isLocale(fromHeader)
      ? fromHeader
      : defaultLocale;
  return { locale, messages: (await import(`../messages/${locale}.json`)).default };
});
