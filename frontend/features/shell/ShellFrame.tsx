"use client";

import { X } from "lucide-react";
import { usePathname } from "next/navigation";
import { useMemo, useState, type MouseEvent, type ReactNode } from "react";

import { BrandMark } from "@/components/BrandMark";
import { Button } from "@/components/ui/Button";
import { Sheet, SheetClose, SheetContent, SheetTitle } from "@/components/ui/Sheet";
import { useIsBelowMd } from "@/hooks/useMediaQuery";

import { MenuButton } from "./MenuButton";
import { pageOwnsHeader, ShellMenuContext, type ShellMenu } from "./shell-menu";
import { useSidebarRail } from "./useSidebarRail";

type Props = {
  sidebar: ReactNode;
  initialCollapsed: boolean;
  labels: { openMenu: string; closeMenu: string; collapse: string; expand: string; drawerTitle: string };
  children: ReactNode;
};

/**
 * Desktop: the sidebar sits on the left, full or folded to an icon rail (it reads `data-collapsed` through the
 * `group/shell` variant). Below `md` the same sidebar opens in a left Sheet; pages with their own header carry
 * the menu button, other pages get a 56px top bar with the menu button and the brand.
 */
export function ShellFrame({ sidebar, initialCollapsed, labels, children }: Props) {
  const pathname = usePathname() ?? "";
  const belowMd = useIsBelowMd();
  const rail = useSidebarRail(initialCollapsed);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [shownPath, setShownPath] = useState(pathname);
  // Navigating from inside the drawer should land on the page, not leave the drawer covering it; growing past
  // md has no drawer to keep open.
  if (pathname !== shownPath || (drawerOpen && !belowMd)) {
    setShownPath(pathname);
    setDrawerOpen(false);
  }

  const menu = useMemo<ShellMenu>(
    () => ({
      open: () => setDrawerOpen(true),
      label: labels.openMenu,
      expanded: drawerOpen,
      collapsed: rail.collapsed,
      toggleCollapsed: rail.toggleCollapsed,
      collapseLabel: labels.collapse,
      expandLabel: labels.expand,
    }),
    [labels.openMenu, labels.collapse, labels.expand, drawerOpen, rail.collapsed, rail.toggleCollapsed],
  );

  // A link inside the drawer (the menus portal out of it, but React events still bubble) ends the visit.
  function closeOnLink(event: MouseEvent) {
    if ((event.target as HTMLElement).closest("a")) setDrawerOpen(false);
  }

  return (
    <ShellMenuContext.Provider value={menu}>
      <div className="group/shell flex h-dvh overflow-hidden" data-collapsed={rail.collapsed}>
        <div className="hidden md:flex">{sidebar}</div>
        <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
          <SheetContent
            side="left"
            showCloseButton={false}
            aria-describedby={undefined}
            onClick={closeOnLink}
            className="bg-sidebar text-sidebar-foreground w-64 gap-0 border-r-0 p-0 md:hidden"
          >
            <SheetTitle className="sr-only">{labels.drawerTitle}</SheetTitle>
            {sidebar}
            <SheetClose asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label={labels.closeMenu}
                title={labels.closeMenu}
                className="absolute top-1.5 right-3"
              >
                <X aria-hidden />
              </Button>
            </SheetClose>
          </SheetContent>
        </Sheet>
        <div className="flex min-w-0 flex-1 flex-col">
          {!pageOwnsHeader(pathname) && (
            <header className="bg-sidebar border-border flex h-14 shrink-0 items-center gap-2 border-b px-2 md:hidden">
              <MenuButton />
              <BrandMark label="ThomasAgent" />
            </header>
          )}
          <main className="min-h-0 min-w-0 flex-1">{children}</main>
        </div>
      </div>
    </ShellMenuContext.Provider>
  );
}
