import type { LucideIcon } from "lucide-react";
import { Search, SquarePen } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { BrandMark } from "@/components/metro/BrandMark";
import { LineDots } from "@/components/metro/LineDots";
import type { Line, ToolLine } from "@/components/metro/lines";
import type { SessionUser } from "@/features/auth/types";
import { cn } from "@/lib/cn";

import { NavLink } from "./NavLink";

export type NavItem = { href: string; label: string; icon: LucideIcon; lines?: Line[] };
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
  className?: string;
};

const NEW_CHAT_HREF = "/chat";
const rowLink =
  "hover:bg-sign-hover aria-[current=page]:bg-sign-active aria-[current=page]:text-sign-ink flex items-center rounded-md transition-colors";

function Recents({ recents, labels, activeHref }: Pick<Props, "recents" | "labels" | "activeHref">) {
  return (
    <>
      <h2 className="text-sign-ink-2 mx-2.5 mt-5.5 mb-2 text-xs font-semibold">{labels.recents}</h2>
      <ul className="-mx-1 flex min-h-0 flex-1 flex-col gap-px overflow-y-auto px-1">
        {recents.length === 0 && (
          <li className="text-sign-ink-2 px-2.5 text-xs leading-relaxed">{labels.recentsEmpty}</li>
        )}
        {recents.map((r) => {
          const href = `/chat/${r.id}`;
          return (
            <li key={r.id}>
              <NavLink
                href={href}
                match="exact"
                serverPath={activeHref}
                className={cn(
                  rowLink,
                  "text-sign-ink/85 grid grid-cols-[1fr_auto] gap-2 px-2.5 py-1.5 text-[13.5px]",
                )}
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

/** The enamel-black station sign: brand, primary actions, lines, recents and the owner plate. */
export function Sidebar({ items, recents, user, labels, activeHref, footer, className }: Props) {
  const name = user.displayName || user.email;
  return (
    <aside
      className={cn(
        "bg-sign text-sign-ink flex h-dvh w-62 shrink-0 flex-col px-3.5 pt-4.5 pb-3.5",
        className,
      )}
    >
      <BrandMark label="ThomasAgent" className="px-1.5 pb-4.5" />
      <Link
        href={NEW_CHAT_HREF}
        className="bg-sign-ink text-sign flex items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm font-semibold transition-opacity hover:opacity-90"
      >
        <SquarePen aria-hidden className="size-4" />
        {labels.newChat}
      </Link>
      <button
        type="button"
        disabled
        title={labels.searchSoon}
        className="border-sign-rule text-sign-ink-2 mt-2 flex cursor-not-allowed items-center gap-2.5 rounded-lg border px-3 py-2 text-sm"
      >
        <Search aria-hidden className="size-4" />
        {labels.search}
        <kbd className="ml-auto font-mono text-[11px]">⌘K</kbd>
      </button>
      <nav aria-label={labels.nav} className="mt-4.5 flex flex-col gap-0.5">
        {items.map(({ href, label, icon: Icon, lines }) => (
          <NavLink
            key={href}
            href={href}
            match="section"
            serverPath={activeHref}
            className={cn(rowLink, "gap-2.5 px-2.5 py-2 text-sm")}
          >
            <Icon aria-hidden className="size-4" />
            {label}
            {lines && (
              <span className="ml-auto">
                <LineDots lines={lines} size="md" />
              </span>
            )}
          </NavLink>
        ))}
      </nav>
      <Recents recents={recents} labels={labels} activeHref={activeHref} />
      <div className="border-sign-rule mt-3 flex items-center gap-2.5 border-t px-2 pt-2.5">
        <span
          className="bg-sign-active grid size-7.5 shrink-0 place-items-center rounded-full text-[13px] font-bold"
          aria-hidden
        >
          {name.slice(0, 1).toUpperCase()}
        </span>
        <span className="min-w-0 text-sm leading-tight">
          <span className="block truncate">{name}</span>
          <small className="text-sign-ink-2 block truncate text-xs">{labels.tenant}</small>
        </span>
        {footer}
      </div>
    </aside>
  );
}
