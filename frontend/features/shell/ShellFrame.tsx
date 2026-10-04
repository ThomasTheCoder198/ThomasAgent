"use client";

import { X } from "lucide-react";
import { usePathname } from "next/navigation";
import { useCallback, useMemo, useState, type ReactNode } from "react";

import { BrandMark } from "@/components/metro/BrandMark";
import { IconButton } from "@/components/ui/Button";
import { useDismiss } from "@/components/ui/useDismiss";
import { cn } from "@/lib/cn";

import { MenuButton } from "./MenuButton";
import { OWN_HEADER_PREFIX, ShellMenuContext } from "./shell-menu";

type Props = { sidebar: ReactNode; labels: { openMenu: string; closeMenu: string }; children: ReactNode };

/**
 * Desktop: the sign sits fixed on the left. Below `md` it becomes an off-canvas drawer. Pages with their
 * own header (chat) put the menu button in it; other pages get a slim sign bar.
 */
export function ShellFrame({ sidebar, labels, children }: Props) {
  const [open, setOpen] = useState(false);
  const pathname = usePathname() ?? "";
  const [shownPath, setShownPath] = useState(pathname);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, close);
  // Navigating from inside the drawer should land on the page, not leave the drawer covering it.
  if (pathname !== shownPath) {
    setShownPath(pathname);
    setOpen(false);
  }
  const menu = useMemo(
    () => ({ open: () => setOpen(true), label: labels.openMenu, expanded: open }),
    [labels.openMenu, open],
  );
  const pageOwnsHeader = pathname.startsWith(OWN_HEADER_PREFIX);

  return (
    <ShellMenuContext.Provider value={menu}>
      <div className="flex h-dvh overflow-hidden">
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
        {open && <div className="bg-scrim fixed inset-0 z-30 md:hidden" aria-hidden onClick={close} />}
        <div className="flex min-w-0 flex-1 flex-col">
          {!pageOwnsHeader && (
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
