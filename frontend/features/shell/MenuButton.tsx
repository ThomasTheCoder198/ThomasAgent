"use client";

import { Menu } from "lucide-react";

import { IconButton } from "@/components/ui/Button";

import { useShellMenu } from "./shell-menu";

/** Opens the mobile sidebar drawer; renders nothing on desktop or outside the workspace shell. */
export function MenuButton({ tone = "ground" }: { tone?: "ground" | "sign" }) {
  const menu = useShellMenu();
  if (!menu) return null;
  return (
    <IconButton
      tone={tone}
      label={menu.label}
      aria-expanded={menu.expanded}
      aria-controls="workspace-sidebar"
      onClick={menu.open}
      className="-ml-1.5 md:hidden"
    >
      <Menu aria-hidden className="size-4.5" />
    </IconButton>
  );
}
