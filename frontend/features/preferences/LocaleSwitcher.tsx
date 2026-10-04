"use client";

import { useLocale, useTranslations } from "next-intl";
import { useTransition } from "react";

import { Segmented } from "@/components/ui/Segmented";
import { locales, type Locale } from "@/i18n/config";

import { setLocale } from "./actions";

export function LocaleSwitcher() {
  const t = useTranslations("preferences");
  const current = useLocale() as Locale;
  const [pending, startTransition] = useTransition();
  return (
    <Segmented<Locale>
      name="locale"
      legend={t("language")}
      value={current}
      disabled={pending}
      onChange={(locale) => startTransition(() => setLocale(locale))}
      options={locales.map((value) => ({ value, label: t(value) }))}
    />
  );
}
