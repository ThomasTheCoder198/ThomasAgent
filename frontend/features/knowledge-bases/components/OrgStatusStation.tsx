"use client";

import { CircleAlert, Clock } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ThinkingOrb } from "thinking-orbs";

import { Button } from "@/components/ui/Button";
import { retryOrganization } from "@/features/orgs/api";
import type { Organization } from "@/features/orgs/contract";
import { MenuButton } from "@/features/shell/MenuButton";
import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import { FILE_POLL_INTERVAL_MS, ORG_PENDING_MAX_REFRESHES } from "../contract";
import { STATUS_ORB_SIZE } from "../layout";

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
  const locale = useLocale() as Locale;
  const { refresh } = useRouter();
  const notify = useNotify();
  const [pending, setPending] = useState(false);

  async function retry() {
    setPending(true);
    try {
      const provisioned = await retryOrganization(org.id, locale);
      // Provisioning answers with the org's real state: a failure is the station's own red block, not a toast.
      if (provisioned.ragxStatus === "ready") notify.success(t("toast.orgReady", { name: org.name }));
      refresh();
    } catch (err) {
      notify.error(err);
    } finally {
      setPending(false);
    }
  }
  return { retry, pending };
}

/** A 48px roundel: the live orb while ragx works, a muted clock once it stalled, a destructive mark on failure. */
function StatusMark({ failed, live }: { failed: boolean; live: boolean }) {
  return (
    <span aria-hidden className="bg-secondary grid size-12 place-items-center rounded-full">
      {live && <ThinkingOrb state="connecting" size={STATUS_ORB_SIZE} aria-hidden />}
      {failed && <CircleAlert className="text-destructive size-8" />}
      {!failed && !live && <Clock className="text-muted-foreground size-8" />}
    </span>
  );
}

/**
 * The org is not usable yet: one centred status block. Provisioning is synchronous, so `pending` means a call
 * died midway; it refreshes for a while, then offers Retry (a `failed` org, with its code, offers it at once).
 */
export function OrgStatusStation({ org }: { org: Organization }) {
  const t = useTranslations("kb");
  const failed = org.ragxStatus === "failed";
  const stalled = useBoundedRefresh(!failed);
  const { retry, pending } = useRetry(org);
  return (
    <div className="h-full overflow-y-auto">
      <div className="px-4 pt-4 md:hidden">
        <MenuButton />
      </div>
      <section
        role="status"
        className="mx-auto flex w-full max-w-md flex-col items-center gap-3 px-4 py-12 text-center md:py-24"
      >
        <StatusMark failed={failed} live={!failed && !stalled} />
        <h1 className="text-lg leading-snug font-semibold tracking-[-0.01em]">
          {failed ? t("orgFailed") : t("orgPending")}
        </h1>
        <p className="text-muted-foreground font-medium">{org.name}</p>
        {failed && org.ragxErrorCode && (
          <p className="tabular text-destructive text-xs">{org.ragxErrorCode}</p>
        )}
        {stalled && <p className="text-muted-foreground">{t("orgPendingStalled")}</p>}
        {(failed || stalled) && (
          <Button onClick={() => void retry()} pending={pending} className="mt-3">
            {t("retryOrg")}
          </Button>
        )}
      </section>
    </div>
  );
}
