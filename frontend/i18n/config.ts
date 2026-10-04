export const LOCALE_COOKIE = "NEXT_LOCALE";
export const locales = ["vi", "en"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "vi";

export function isLocale(value: string | undefined): value is Locale {
  return locales.includes(value as Locale);
}
