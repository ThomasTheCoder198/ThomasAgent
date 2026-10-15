import { getTranslations } from "next-intl/server";
import type { ReactNode } from "react";

import { Skeleton } from "@/components/ui/Skeleton";
import { MenuButton } from "@/features/shell/MenuButton";

import { PAGE_COLUMN } from "../layout";
import { KbPageHeaderSkeleton } from "./KbPageHeader";

const LIST_ROWS = 5;
const FILE_ROWS = 4;

function RowSkeleton() {
  return (
    <li className="border-border flex min-h-14 items-center justify-between gap-6 border-b px-3 py-3">
      <Skeleton className="h-4 w-48 max-w-1/2" />
      <Skeleton className="h-4 w-24" />
    </li>
  );
}

/** Announced once, as a status: the shapes inside are decoration. */
async function LoadingStatus({ children }: { children: ReactNode }) {
  const t = await getTranslations("common");
  return (
    <div role="status" aria-busy="true" className="h-full overflow-y-auto">
      <span className="sr-only">{t("loading")}</span>
      {children}
    </div>
  );
}

/** The shell PageHeader's footprint: menu button on phones, title, description, the New button. */
function ListHeaderSkeleton() {
  return (
    <div className="mx-auto flex w-full max-w-5xl items-start gap-3 px-4 pt-6 pb-6 md:px-6 md:pt-10 md:pb-8 lg:px-8">
      <MenuButton />
      <div className="flex min-w-0 flex-1 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 flex-col gap-3">
          <Skeleton className="h-8 w-56 max-w-full" />
          <Skeleton className="h-4 w-80 max-w-full" />
        </div>
        <Skeleton className="h-9 w-44" />
      </div>
    </div>
  );
}

/** Header and ruled rows in the list's own proportions. */
export function KnowledgeBaseListSkeleton() {
  return (
    <LoadingStatus>
      <ListHeaderSkeleton />
      <div aria-hidden className={PAGE_COLUMN}>
        <ul className="border-border border-t">
          {Array.from({ length: LIST_ROWS }, (_, index) => (
            <RowSkeleton key={index} />
          ))}
        </ul>
      </div>
    </LoadingStatus>
  );
}

/** Header, the drop zone and file rows. */
export function KnowledgeBasePageSkeleton() {
  return (
    <LoadingStatus>
      <KbPageHeaderSkeleton />
      <div aria-hidden className={PAGE_COLUMN}>
        <Skeleton className="h-40 w-full" />
        <ul className="border-border mt-6 border-t">
          {Array.from({ length: FILE_ROWS }, (_, index) => (
            <RowSkeleton key={index} />
          ))}
        </ul>
      </div>
    </LoadingStatus>
  );
}
