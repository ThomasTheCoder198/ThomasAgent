import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Skeleton } from "@/components/ui/Skeleton";
import { MenuButton } from "@/features/shell/MenuButton";
import { cn } from "@/lib/cn";

import { PAGE_COLUMN } from "../layout";

type Props = {
  title: string;
  back: { href: string; label: string };
  /** Facts about the page's subject (badges, counts, mono data). */
  meta?: ReactNode;
  actions?: ReactNode;
};

/**
 * The detail screens' editorial header, the shell PageHeader's sibling for pages that lead with a way back:
 * a back row (carrying the menu button on phones, because `/orgs` pages own their header), the Newsreader
 * title, a meta row, and actions on the right (under the title below sm).
 */
export function KbPageHeader({ title, back, meta, actions }: Props) {
  return (
    <header className={cn(PAGE_COLUMN, "pt-4 pb-6 md:pt-8")}>
      <div className="flex items-center gap-2">
        <MenuButton />
        <Link
          href={back.href}
          className="text-muted-foreground hover:text-foreground inline-flex min-h-11 items-center gap-1.5 text-sm font-medium transition-colors md:min-h-8"
        >
          <ArrowLeft aria-hidden className="size-4" />
          {back.label}
        </Link>
      </div>
      <div className="mt-2 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
        <div className="min-w-0">
          <h1 className="font-serif text-[clamp(1.75rem,3vw,2rem)] leading-tight font-medium tracking-tight [overflow-wrap:anywhere]">
            {title}
          </h1>
          {meta && <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2">{meta}</div>}
        </div>
        {actions && <div className="flex flex-wrap gap-2 sm:shrink-0">{actions}</div>}
      </div>
    </header>
  );
}

/** Same footprint as the header, so the page does not jump when the data lands. */
export function KbPageHeaderSkeleton() {
  return (
    <div className={cn(PAGE_COLUMN, "pt-4 pb-6 md:pt-8")}>
      <div className="flex min-h-11 items-center gap-2 md:min-h-8">
        <MenuButton />
        <Skeleton className="h-4 w-32" />
      </div>
      <div className="mt-2 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
        <div className="flex min-w-0 flex-col gap-3">
          <Skeleton className="h-8 w-56 max-w-full" />
          <Skeleton className="h-4 w-72 max-w-full" />
        </div>
        <div className="flex gap-2">
          <Skeleton className="h-9 w-24" />
          <Skeleton className="h-9 w-20" />
        </div>
      </div>
    </div>
  );
}
