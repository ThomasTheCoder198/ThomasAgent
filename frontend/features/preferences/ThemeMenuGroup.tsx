"use client";

import { Monitor, Moon, Sun } from "lucide-react";
import { useTranslations } from "next-intl";
import { useTheme } from "next-themes";

import {
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/DropdownMenu";

const themes = ["light", "dark", "system"] as const;
type Theme = (typeof themes)[number];
const icons = { light: Sun, dark: Moon, system: Monitor } as const;

function isTheme(value: string | undefined): value is Theme {
  return themes.some((theme) => theme === value);
}

/** Theme choice as radio items; render it inside a `DropdownMenuContent`. */
export function ThemeMenuGroup() {
  const t = useTranslations("preferences");
  const { theme, setTheme } = useTheme();
  return (
    <>
      <DropdownMenuLabel>{t("theme")}</DropdownMenuLabel>
      <DropdownMenuRadioGroup
        value={isTheme(theme) ? theme : ""}
        onValueChange={(next) => isTheme(next) && setTheme(next)}
      >
        {themes.map((value) => {
          const Icon = icons[value];
          return (
            <DropdownMenuRadioItem key={value} value={value}>
              <Icon aria-hidden />
              {t(value)}
            </DropdownMenuRadioItem>
          );
        })}
      </DropdownMenuRadioGroup>
    </>
  );
}
