"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import type { Locale } from "@/i18n/config";
import { ApiError, apiFetch } from "@/lib/api/client";

import { AGENT_PATHS, type AgentDetail } from "./contract";
import {
  canSave as canSaveDraft,
  diffDraft,
  draftErrors,
  fromDetail,
  isDirty,
  type AgentDraft,
} from "./draft";

export type SaveState =
  { status: "idle" } | { status: "saving" } | { status: "saved" } | { status: "error"; message: string };

const SAVED_FLASH_MS = 2200;

/** The "Saved" confirmation fades after a moment; its timer dies with the next save or the component. */
function useSavedFlash(onExpire: () => void) {
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  return {
    cancel: () => clearTimeout(timer.current),
    start: () => {
      clearTimeout(timer.current);
      timer.current = setTimeout(onExpire, SAVED_FLASH_MS);
    },
  };
}

/** Sends only what changed; the server answers with the stored Agent. */
function patchAgent(agent: AgentDetail, draft: AgentDraft, locale: Locale) {
  return apiFetch<AgentDetail>(AGENT_PATHS.agent(agent.id), {
    method: "PATCH",
    locale,
    body: JSON.stringify(diffDraft(agent, draft)),
  });
}

/**
 * Local draft of one Agent plus the PATCH that persists exactly what changed. While a save is in flight the
 * draft is frozen (the form is disabled and `update` is ignored), so the server's answer can replace the
 * draft without dropping anything typed after the snapshot was sent.
 */
export function useAgentEditor(initial: AgentDetail) {
  const locale = useLocale() as Locale;
  const tErr = useTranslations("errors");
  const router = useRouter();
  const [agent, setAgent] = useState(initial);
  const [draft, setDraft] = useState<AgentDraft>(() => fromDetail(initial));
  const [save, setSave] = useState<SaveState>({ status: "idle" });
  const savingRef = useRef(false);
  const flash = useSavedFlash(() => setSave((s) => (s.status === "saved" ? { status: "idle" } : s)));

  function update(change: Partial<AgentDraft>) {
    if (savingRef.current) return;
    setDraft((current) => ({ ...current, ...change }));
    setSave((s) => (s.status === "saving" ? s : { status: "idle" }));
  }

  function discard() {
    if (savingRef.current) return;
    setDraft(fromDetail(agent));
    setSave({ status: "idle" });
  }

  async function persist() {
    if (savingRef.current || !canSaveDraft(agent, draft)) return;
    savingRef.current = true;
    flash.cancel();
    setSave({ status: "saving" });
    try {
      const saved = await patchAgent(agent, draft, locale);
      setAgent(saved);
      setDraft(fromDetail(saved));
      setSave({ status: "saved" });
      router.refresh();
      flash.start();
    } catch (err) {
      setSave({ status: "error", message: err instanceof ApiError ? err.message : tErr("generic") });
    } finally {
      savingRef.current = false;
    }
  }

  return {
    agent,
    draft,
    errors: draftErrors(draft),
    dirty: isDirty(agent, draft),
    canSave: canSaveDraft(agent, draft),
    saving: save.status === "saving",
    save,
    update,
    discard,
    persist,
  };
}
