"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import { saveRoles, syncProvider } from "./api";
import type { SettingsScope } from "./contract";
import { planSave, useRoleDraft } from "./useRoleDraft";

export type SettingsTab = "platform" | "org";
type Draft = ReturnType<typeof useRoleDraft>;

/** The active scope tab; leaving it with unsaved edits waits for the discard prompt's answer. */
function useScopeTabs(orgId: string | null, draft: Draft) {
  const [tab, setTab] = useState<SettingsTab>("platform");
  const [pendingTab, setPendingTab] = useState<SettingsTab | null>(null);
  const [resetKey, setResetKey] = useState(0);

  function discard() {
    draft.reset();
    // Rows keep local state (an override in progress); a new key makes them start over.
    setResetKey((n) => n + 1);
  }

  return {
    tab,
    pendingTab,
    resetKey,
    scope: (tab === "org" && orgId ? { scope: "org", orgId } : { scope: "platform" }) as SettingsScope,
    /** The org tab has nothing to edit without an org. */
    editable: tab === "platform" || orgId !== null,
    discard,
    changeTab(next: SettingsTab) {
      if (next === tab) return;
      if (draft.dirty) setPendingTab(next);
      else setTab(next);
    },
    cancelTabChange: () => setPendingTab(null),
    confirmTabChange() {
      discard();
      if (pendingTab) setTab(pendingTab);
      setPendingTab(null);
    },
  };
}

/** If the refreshed data never arrives, the page stops waiting and unlocks Save. */
const REFRESH_WAIT_MS = 10_000;

/**
 * Save and sync each report their outcome as a toast and refresh the server data. A successful save keeps the
 * draft (and the Save button busy) until the refreshed roles arrive, so the rows never flash back to the old value.
 */
function useSettingsActions(
  scope: SettingsScope,
  draft: Draft,
  fresh: { platformRoles: unknown; orgRoles: unknown },
) {
  const t = useTranslations("settings");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [saving, setSaving] = useState(false);
  const awaitingRefresh = useRef(false);
  const { reset } = draft;

  // The roles change identity only when a refresh delivers new props.
  useEffect(() => {
    if (!awaitingRefresh.current) return;
    awaitingRefresh.current = false;
    reset();
    setSaving(false);
  }, [reset, fresh.platformRoles, fresh.orgRoles]);

  async function save() {
    setSaving(true);
    try {
      await saveRoles(planSave(scope, draft.draft), locale);
      notify.success(t("saved"));
      awaitingRefresh.current = true;
      window.setTimeout(() => {
        if (!awaitingRefresh.current) return;
        awaitingRefresh.current = false;
        reset();
        setSaving(false);
      }, REFRESH_WAIT_MS);
    } catch (err) {
      // Earlier requests of the batch are stored; the draft stays so the operator can retry the rest.
      notify.error(err);
      setSaving(false);
    }
    router.refresh();
  }

  async function syncOne(providerId: string) {
    try {
      const result = await syncProvider(providerId, locale);
      notify.success(t("synced", { added: result.added, updated: result.updated }));
      router.refresh();
    } catch (err) {
      notify.error(err);
    }
  }

  return { saving, save, syncOne };
}

/** The page's state: the scope tab, the draft that belongs to it, and the save / sync calls. */
export function useSettingsPage(orgId: string | null, fresh: { platformRoles: unknown; orgRoles: unknown }) {
  const draft = useRoleDraft();
  const tabs = useScopeTabs(orgId, draft);
  const actions = useSettingsActions(tabs.scope, draft, fresh);
  return { draft, ...tabs, ...actions };
}
