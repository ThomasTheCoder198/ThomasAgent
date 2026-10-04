"use client";

import { Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Segmented } from "@/components/ui/Segmented";

export type ThemeLabels = { light: string; dark: string; system: string; legend: string };

const themes = ["light", "dark", "system"] as const;
type Theme = (typeof themes)[number];
const icons = { light: Sun, dark: Moon, system: Monitor } as const;

export function ThemeSwitcher({ labels }: { labels: ThemeLabels }) {
  const { theme, setTheme } = useTheme();
  return (
    <Segmented<Theme>
      name="theme"
      legend={labels.legend}
      value={theme as Theme | undefined}
      onChange={setTheme}
      options={themes.map((value) => ({ value, label: labels[value], icon: icons[value] }))}
    />
  );
}
