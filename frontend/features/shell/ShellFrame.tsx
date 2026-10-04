"use client";

import { X } from "lucide-react";
import type { ReactNode } from "react";

import { BrandMark } from "@/components/metro/BrandMark";
import { IconButton } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import { MenuButton } from "./MenuButton";
import { pageOwnsHeader, ShellMenuContext } from "./shell-menu";
import { useShellState } from "./useShellState";

type Props = {
  sidebar: ReactNode;
  initialCollapsed: boolean;
  labels: { openMenu: string; closeMenu: string; collapse: string; expand: string };
  children: ReactNode;
};

/**
 * Desktop: the sign sits on the left, full or folded to an icon rail. Below `md` it becomes an off-canvas
 * drawer; pages with their own header (chat) carry the menu button, other pages get a slim sign bar.
 * The sidebar reads `data-collapsed` through the `group/shell` variant.
 */
export function ShellFrame({ sidebar, initialCollapsed, labels, children }: Props) {
  const { open, close, collapsed, pathname, menu } = useShellState(initialCollapsed, labels);
  const ownsHeader = pageOwnsHeader(pathname);

  return (
    <ShellMenuContext.Provider value={menu}>
      <div className="group/shell flex h-dvh overflow-hidden" data-collapsed={collapsed}>
        <div
          id="workspace-sidebar"
          className={cn(
            "ease-out-expo fixed inset-y-0 left-0 z-40 transition-transform duration-300 md:static md:translate-x-0",
            open ? "translate-x-0" : "-translate-x-full",
          )}
        >
          {sidebar}
          {open && (
            <IconButton
              tone="sign"
              label={labels.closeMenu}
              onClick={close}
              className="absolute top-4 right-3 md:hidden"
            >
              <X aria-hidden className="size-4" />
            </IconButton>
          )}
        </div>
        {open && (
          <div className="bg-scrim animate-fade fixed inset-0 z-30 md:hidden" aria-hidden onClick={close} />
        )}
        <div className="flex min-w-0 flex-1 flex-col">
          {!ownsHeader && (
            <div className="bg-sign text-sign-ink flex h-12 shrink-0 items-center gap-2 px-2 md:hidden">
              <MenuButton tone="sign" />
              <BrandMark label="ThomasAgent" className="scale-90" />
            </div>
          )}
          <main className="min-h-0 min-w-0 flex-1">{children}</main>
        </div>
      </div>
    </ShellMenuContext.Provider>
  );
}
