"use client";

import { Settings } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";

import { LocaleMenuGroup } from "./LocaleMenuGroup";
import { ThemeMenuGroup } from "./ThemeMenuGroup";

/** Theme and language for pages that have no shell menu (sign-in). */
export function PreferencesMenu() {
  const t = useTranslations("preferences");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("menu")} title={t("menu")}>
          <Settings aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <ThemeMenuGroup />
        <DropdownMenuSeparator />
        <LocaleMenuGroup />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
