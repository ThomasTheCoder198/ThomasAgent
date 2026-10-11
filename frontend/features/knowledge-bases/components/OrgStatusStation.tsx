"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ThinkingOrb } from "thinking-orbs";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { retryOrganization } from "@/features/orgs/api";
import type { Organization } from "@/features/orgs/contract";
import { MenuButton } from "@/features/shell/MenuButton";
import type { Locale } from "@/i18n/config";
import { describeApiError } from "@/lib/api/client";
import { cn } from "@/lib/cn";

import { FILE_POLL_INTERVAL_MS, ORG_PENDING_MAX_REFRESHES } from "../contract";

const ORB_SIZE = 20;

/** Refreshes a few times, then reports `stalled`: an endless loading state after a dead call helps nobody. */
function useBoundedRefresh(active: boolean): boolean {
  const { refresh } = useRouter();
  const [stalled, setStalled] = useState(false);
  useEffect(() => {
    if (!active || stalled) return;
    let refreshes = 0;
    const timer = setInterval(() => {
      refreshes += 1;
      refresh();
      if (refreshes >= ORG_PENDING_MAX_REFRESHES) {
        clearInterval(timer);
        setStalled(true);
      }
    }, FILE_POLL_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [active, refresh, stalled]);
  return stalled;
}

function useRetry(org: Organization) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const locale = useLocale() as Locale;
  const { refresh } = useRouter();
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<string>();

  async function retry() {
    setPending(true);
    setFailure(undefined);
    try {
      await retryOrganization(org.id, locale);
      refresh();
    } catch (err) {
      setFailure(describeApiError(err, t("errors.retryOrgFailed"), (id) => tErr("requestId", { id })));
    } finally {
      setPending(false);
    }
  }
  return { retry, pending, failure };
}

function Ring({ failed, live }: { failed: boolean; live: boolean }) {
  if (live) {
    return (
      <span className="border-kb bg-kb-tint grid size-7 place-items-center rounded-full border-[3px]">
        <ThinkingOrb state="connecting" size={ORB_SIZE} />
      </span>
    );
  }
  return (
    <span
      className={cn("bg-paper size-5.5 rounded-full border-[5px]", failed ? "border-app" : "border-todo")}
    />
  );
}

/**
 * The org is not usable yet: one station on the KB line. Provisioning is synchronous, so `pending` means a call
 * died midway; it still refreshes for a while, and Retry is always there (the org may be `failed` with a code).
 */
export function OrgStatusStation({ org }: { org: Organization }) {
  const t = useTranslations("kb");
  const failed = org.ragxStatus === "failed";
  const stalled = useBoundedRefresh(!failed);
  const { retry, pending, failure } = useRetry(org);
  return (
    <div className="h-full overflow-y-auto">
      <div className="px-4 pt-4 md:hidden">
        <MenuButton />
      </div>
      <section
        role="status"
        className="mx-auto flex w-full max-w-[68ch] items-start gap-5 px-4 py-14 md:px-8 md:py-20"
      >
        <span className="flex shrink-0 flex-col items-center pt-1" aria-hidden>
          <Ring failed={failed} live={!failed && !stalled} />
          <span className={cn("mt-0.5 h-16 w-[3px] rounded-b-sm", failed ? "bg-app" : "bg-kb")} />
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="text-[22px] font-bold tracking-[-0.015em]">
            {failed ? t("orgFailed") : t("orgPending")}
          </h1>
          <p className="text-ink-2 mt-2 font-medium">{org.name}</p>
          {failed && org.ragxErrorCode && (
            <p className="tabular text-app mt-1 text-xs">{org.ragxErrorCode}</p>
          )}
          {stalled && <p className="text-ink-2 mt-2 leading-relaxed">{t("orgPendingStalled")}</p>}
          {failure && (
            <div className="mt-4">
              <InlineAlert title={failure} />
            </div>
          )}
          <Button onClick={() => void retry()} pending={pending} disabled={pending} className="mt-6">
            {t("retryOrg")}
          </Button>
        </div>
      </section>
    </div>
  );
}
