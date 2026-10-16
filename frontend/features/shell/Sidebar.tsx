import type { LucideIcon } from "lucide-react";
import { Search, SquarePen } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { BrandMark } from "@/components/BrandMark";
import { LineDots } from "@/components/LineDots";
import type { Line, ToolLine } from "@/components/lines";
import { Button } from "@/components/ui/Button";
import type { SessionUser } from "@/features/auth/types";
import { cn } from "@/lib/cn";

import { NavGroup } from "./NavGroup";
import { NavLink, type OrgScope } from "./NavLink";

export type NavItem = {
  href: string;
  label: string;
  icon: LucideIcon;
  lines?: Line[];
  /** Makes `href` follow the org in the URL, with the server-resolved org as fallback. */
  orgScope?: OrgScope;
};
export type RecentItem = { id: string; title: string; lines: ToolLine[] };
export type SidebarLabels = {
  newChat: string;
  search: string;
  searchSoon: string;
  recents: string;
  recentsEmpty: string;
  tenant: string;
  nav: string;
};

type Props = {
  items: NavItem[];
  recents: RecentItem[];
  user: SessionUser;
  labels: SidebarLabels;
  activeHref: string;
  footer?: ReactNode;
  /** The org switcher, under the brand. */
  org?: ReactNode;
  /** Desktop collapse control, rendered beside the brand. */
  toggle?: ReactNode;
  className?: string;
};

const NEW_CHAT_HREF = "/chat";
/** 40px rows (44px below md); the current page is a card with a 1px outline, a violet icon and a sliding violet bar. */
const rowLink =
  "group/row hover:bg-sidebar-accent aria-[current=page]:bg-card aria-[current=page]:border-outline relative flex min-h-10 items-center rounded-md border border-transparent text-sm transition-colors duration-150 max-md:min-h-11";

/** Folded to the icon rail (desktop only): text stays for screen readers, decoration goes. */
const RAIL_TEXT = "md:group-data-[collapsed=true]/shell:sr-only";
const RAIL_GONE = "md:group-data-[collapsed=true]/shell:hidden";
const RAIL_CENTER =
  "md:group-data-[collapsed=true]/shell:justify-center md:group-data-[collapsed=true]/shell:px-0";

function Recents({ recents, labels, activeHref }: Pick<Props, "recents" | "labels" | "activeHref">) {
  return (
    <>
      <h2 className="text-muted-foreground px-3 pt-6 pb-2 text-[0.8125rem] font-medium">{labels.recents}</h2>
      <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
        {recents.length === 0 && (
          <li className="text-muted-foreground px-3 text-[0.8125rem] leading-snug">{labels.recentsEmpty}</li>
        )}
        {recents.map((r) => {
          const href = `/chat/${r.id}`;
          return (
            <li key={r.id}>
              <NavLink
                href={href}
                match="exact"
                serverPath={activeHref}
                className={cn(rowLink, "grid grid-cols-[1fr_auto] gap-2 px-3")}
              >
                <span className="truncate" title={r.title}>
                  {r.title}
                </span>
                <LineDots lines={r.lines} />
              </NavLink>
            </li>
          );
        })}
      </ul>
    </>
  );
}

function PrimaryActions({ labels }: Pick<Props, "labels">) {
  return (
    <div className="flex flex-col gap-2">
      <Button asChild className={cn("w-full", "md:group-data-[collapsed=true]/shell:px-0")}>
        <Link href={NEW_CHAT_HREF} title={labels.newChat}>
          <SquarePen aria-hidden />
          <span className={RAIL_TEXT}>{labels.newChat}</span>
        </Link>
      </Button>
      {/* A disabled button swallows pointer events, so the tooltip hangs on its wrapper. */}
      <span title={labels.searchSoon} className="block">
        <Button
          variant="ghost"
          disabled
          className={cn("text-muted-foreground w-full justify-start px-3", RAIL_CENTER)}
        >
          <Search aria-hidden />
          <span className={RAIL_TEXT}>{labels.search}</span>
          <kbd className={cn("ml-auto font-sans text-xs", RAIL_GONE)}>⌘K</kbd>
        </Button>
      </span>
    </div>
  );
}

function OwnerPlate({ user, labels, footer }: Pick<Props, "user" | "labels" | "footer">) {
  const name = user.displayName || user.email;
  return (
    <div
      className={cn(
        "border-sidebar-border mt-auto flex items-center gap-3 border-t pt-3",
        "md:group-data-[collapsed=true]/shell:flex-col md:group-data-[collapsed=true]/shell:gap-2",
      )}
    >
      <span
        title={name}
        className="bg-sidebar-accent grid size-8 shrink-0 place-items-center rounded-full text-sm font-semibold"
        aria-hidden
      >
        {name.slice(0, 1).toUpperCase()}
      </span>
      <span className={cn("min-w-0 flex-1 text-sm leading-tight", RAIL_TEXT)}>
        <span className="block truncate font-medium">{name}</span>
        <small className="text-muted-foreground block truncate text-xs">{labels.tenant}</small>
      </span>
      {footer}
    </div>
  );
}

/** Brand, org switcher, new chat, navigation, recents and the owner plate on the sidebar surface. */
export function Sidebar({ items, recents, user, labels, activeHref, footer, org, toggle, className }: Props) {
  return (
    <aside
      className={cn(
        "bg-sidebar text-sidebar-foreground border-sidebar-border ease-out-expo flex h-full w-64 shrink-0 flex-col border-r px-3 pb-3 transition-[width] duration-300",
        "md:group-data-[collapsed=true]/shell:w-16",
        className,
      )}
    >
      <div
        className={cn(
          "flex h-14 shrink-0 items-center justify-between gap-2 px-3 max-md:pr-14",
          "md:group-data-[collapsed=true]/shell:h-auto md:group-data-[collapsed=true]/shell:flex-col md:group-data-[collapsed=true]/shell:px-0 md:group-data-[collapsed=true]/shell:py-3",
        )}
      >
        <BrandMark label="ThomasAgent" labelClassName={RAIL_TEXT} />
        {toggle}
      </div>
      {org}
      <PrimaryActions labels={labels} />
      <nav aria-label={labels.nav} className="mt-6 flex flex-col gap-1">
        <NavGroup>
          {items.map(({ href, label, icon: Icon, lines, orgScope }) => (
            <NavLink
              key={href}
              href={href}
              orgScope={orgScope}
              match="section"
              serverPath={activeHref}
              title={label}
              indicator
              className={cn(rowLink, "gap-3 px-3 font-medium", RAIL_CENTER)}
            >
              <Icon
                aria-hidden
                className="group-aria-[current=page]/row:text-primary size-4 shrink-0 transition-colors"
              />
              <span className={RAIL_TEXT}>{label}</span>
              {lines && (
                <span className={cn("ml-auto", RAIL_GONE)}>
                  <LineDots lines={lines} size="md" />
                </span>
              )}
            </NavLink>
          ))}
        </NavGroup>
      </nav>
      <div className={cn("flex min-h-0 flex-1 flex-col", RAIL_GONE)}>
        <Recents recents={recents} labels={labels} activeHref={activeHref} />
      </div>
      <OwnerPlate user={user} labels={labels} footer={footer} />
    </aside>
  );
}
