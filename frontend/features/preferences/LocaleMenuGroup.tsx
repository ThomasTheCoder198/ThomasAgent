"use client";

import { useLocale, useTranslations } from "next-intl";
import { useTransition } from "react";

import {
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/DropdownMenu";
import { isLocale, locales } from "@/i18n/config";

import { setLocale } from "./actions";

/** Language choice as radio items; render it inside a `DropdownMenuContent`. */
export function LocaleMenuGroup() {
  const t = useTranslations("preferences");
  const current = useLocale();
  const [, startTransition] = useTransition();
  return (
    <>
      <DropdownMenuLabel>{t("language")}</DropdownMenuLabel>
      <DropdownMenuRadioGroup
        value={current}
        onValueChange={(next) => isLocale(next) && startTransition(() => setLocale(next))}
      >
        {locales.map((value) => (
          <DropdownMenuRadioItem key={value} value={value}>
            {t(value)}
          </DropdownMenuRadioItem>
        ))}
      </DropdownMenuRadioGroup>
    </>
  );
}
