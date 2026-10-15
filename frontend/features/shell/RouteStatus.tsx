"use client";

import { SearchX, ServerCrash } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { useTransition } from "react";

import { Button } from "@/components/ui/Button";
import { Skeleton } from "@/components/ui/Skeleton";

import { EmptyState } from "./EmptyState";
import { MenuButton } from "./MenuButton";
import { pageOwnsHeader } from "./shell-menu";

type Kind = "loading" | "error" | "notFound";

type Props = {
  kind: Kind;
  workspace?: boolean;
  requestId?: string;
  retry?: () => void;
};

const SKELETON_ROWS = 4;

/** The shape of a workspace page (title, description, a few rows), so loading does not jump into place. */
function PageSkeleton() {
  const t = useTranslations("routeStatus");
  return (
    <div role="status" aria-live="polite" aria-busy="true">
      <span className="sr-only">{t("loadingTitle")}</span>
      <div aria-hidden className="mx-auto w-full max-w-5xl px-4 pt-6 md:px-6 md:pt-10 lg:px-8">
        <Skeleton className="h-8 w-48 md:h-9" />
        <Skeleton className="mt-3 h-4 w-full max-w-md" />
        <div className="mt-8 flex flex-col gap-3">
          {Array.from({ length: SKELETON_ROWS }, (_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      </div>
    </div>
  );
}

function ServiceError({ requestId, retry }: Pick<Props, "requestId" | "retry">) {
  const t = useTranslations("routeStatus");
  const common = useTranslations("common");
  const errors = useTranslations("errors");
  const [pending, startTransition] = useTransition();
  return (
    <div role="alert">
      <EmptyState line="app" icon={ServerCrash} title={t("errorTitle")} body={t("errorBody")}>
        {requestId && (
          <p className="text-muted-foreground tabular text-xs break-all">
            {errors("requestId", { id: requestId })}
          </p>
        )}
        {retry && (
          <Button pending={pending} onClick={() => startTransition(retry)} className="mt-3">
            {common("retry")}
          </Button>
        )}
      </EmptyState>
    </div>
  );
}

function NotFound() {
  const t = useTranslations("routeStatus");
  return (
    <EmptyState line="model" icon={SearchX} title={t("notFoundTitle")} body={t("notFoundBody")}>
      <Button asChild variant="outline" className="mt-3">
        <Link href="/chat">{t("backToWorkspace")}</Link>
      </Button>
    </EmptyState>
  );
}

/**
 * Route boundaries: a skeleton while a page loads, a retryable service error (never a 404) when the backend
 * fails, and a 404 only for a missing route or resource.
 */
export function RouteStatus({ kind, workspace = false, requestId, retry }: Props) {
  const pathname = usePathname();
  const showMenu = workspace && pageOwnsHeader(pathname);
  return (
    <div
      className={
        workspace
          ? "h-full overflow-y-auto"
          : kind === "loading"
            ? "min-h-dvh"
            : "grid min-h-dvh place-items-center"
      }
    >
      {showMenu && (
        <div className="px-4 pt-4 md:hidden">
          <MenuButton />
        </div>
      )}
      {kind === "loading" && <PageSkeleton />}
      {kind === "error" && <ServiceError requestId={requestId} retry={retry} />}
      {kind === "notFound" && <NotFound />}
    </div>
  );
}
