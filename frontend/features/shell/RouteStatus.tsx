"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { useTransition } from "react";

import { Button } from "@/components/ui/Button";

import { MenuButton } from "./MenuButton";
import { pageOwnsHeader } from "./shell-menu";

type Kind = "loading" | "error" | "notFound";

type Props = {
  kind: Kind;
  workspace?: boolean;
  requestId?: string;
  retry?: () => void;
};

function StatusAction({ kind, requestId, retry }: Pick<Props, "kind" | "requestId" | "retry">) {
  const t = useTranslations("routeStatus");
  const common = useTranslations("common");
  const errors = useTranslations("errors");
  const [pending, startTransition] = useTransition();

  if (kind === "loading") {
    return (
      <div className="bg-rule-strong relative mt-6 h-0.5 w-28 overflow-hidden rounded-full" aria-hidden>
        <span className="animate-depart bg-ink absolute inset-y-0 left-0 w-2/5 rounded-full" />
      </div>
    );
  }
  if (kind === "notFound") {
    return (
      <Link
        href="/chat"
        className="text-mcp-ink focus-visible:outline-focus mt-6 inline-flex min-h-10 items-center rounded-lg font-semibold underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        {t("backToWorkspace")}
      </Link>
    );
  }
  return (
    <>
      {requestId && (
        <p className="text-ink-3 tabular mt-3 text-xs break-all">{errors("requestId", { id: requestId })}</p>
      )}
      {retry && (
        <Button disabled={pending} pending={pending} onClick={() => startTransition(retry)} className="mt-6">
          {common("retry")}
        </Button>
      )}
    </>
  );
}

/** Route states keep the Metro station motif without pretending that failed data is an empty list. */
export function RouteStatus({ kind, workspace = false, requestId, retry }: Props) {
  const t = useTranslations("routeStatus");
  const pathname = usePathname();
  const showMenu = workspace && pageOwnsHeader(pathname);
  const status = {
    loading: { title: t("loadingTitle"), body: t("loadingBody"), ring: "border-ink", stem: "bg-ink" },
    error: { title: t("errorTitle"), body: t("errorBody"), ring: "border-app", stem: "bg-app" },
    notFound: { title: t("notFoundTitle"), body: t("notFoundBody"), ring: "border-ink", stem: "bg-ink" },
  }[kind];
  const accessibility = {
    loading: { role: "status", live: "polite" as const, busy: true },
    error: { role: "alert", live: undefined, busy: undefined },
    notFound: { role: undefined, live: undefined, busy: undefined },
  }[kind];

  return (
    <div className={workspace ? "h-full overflow-y-auto" : "min-h-dvh"}>
      {showMenu && (
        <div className="px-4 pt-4 md:hidden">
          <MenuButton />
        </div>
      )}
      <section
        role={accessibility.role}
        aria-live={accessibility.live}
        aria-busy={accessibility.busy}
        className="mx-auto flex w-full max-w-[68ch] items-start gap-5 px-4 py-14 md:px-8 md:py-20"
      >
        <span className="flex shrink-0 flex-col items-center pt-1" aria-hidden>
          <span className={`bg-paper size-5.5 rounded-full border-[5px] ${status.ring}`} />
          <span className={`mt-0.5 h-16 w-[3px] rounded-b-sm ${status.stem}`} />
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="text-[22px] font-bold tracking-[-0.015em]">{status.title}</h1>
          <p className="text-ink-2 mt-2 leading-relaxed">{status.body}</p>
          <StatusAction kind={kind} requestId={requestId} retry={retry} />
        </div>
      </section>
    </div>
  );
}
