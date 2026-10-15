"use client";

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useSyncExternalStore } from "react";

import { Button } from "@/components/ui/Button";

import { useShellMenu } from "./shell-menu";
import { shortcutHint } from "./sidebar-state";

const subscribeNever = () => () => undefined;

/** The platform's own spelling of the shortcut; the server (no platform) renders the generic one. */
function useShortcutHint() {
  return useSyncExternalStore(
    subscribeNever,
    () => shortcutHint(navigator.platform),
    () => shortcutHint(""),
  );
}

/** Desktop-only toggle between the full sign and the icon rail. */
export function CollapseButton() {
  const menu = useShellMenu();
  const hint = useShortcutHint();
  if (!menu) return null;
  const label = menu.collapsed ? menu.expandLabel : menu.collapseLabel;
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={`${label} (${hint})`}
      title={`${label} (${hint})`}
      aria-expanded={!menu.collapsed}
      aria-controls="workspace-sidebar"
      onClick={menu.toggleCollapsed}
      className="hidden md:inline-flex"
    >
      {menu.collapsed ? <PanelLeftOpen aria-hidden /> : <PanelLeftClose aria-hidden />}
    </Button>
  );
}
