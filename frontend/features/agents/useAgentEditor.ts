"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";
import { useNotify } from "@/lib/useNotify";

import type { Agent } from "@/lib/core/types";

import { AGENT_PATHS } from "./contract";
import {
  canSave as canSaveDraft,
  diffDraft,
  draftErrors,
  fromAgent,
  isDirty,
  type AgentDraft,
} from "./draft";

/** Sends only what changed; the server answers with the stored Agent. */
function patchAgent(orgId: string, agent: Agent, draft: AgentDraft, locale: Locale) {
  return apiFetch<Agent>(AGENT_PATHS.agent(orgId, agent.id), {
    method: "PATCH",
    locale,
    body: JSON.stringify(diffDraft(agent, draft)),
  });
}

/**
 * Local draft of one Agent plus the PATCH that persists exactly what changed. While a save is in flight the
 * draft is frozen (the form is disabled and `update` is ignored), so the server's answer can replace the
 * draft without dropping anything typed after the snapshot was sent. The outcome is reported by toast; a
 * failed save keeps the draft, so the owner can retry without retyping.
 */
export function useAgentEditor(orgId: string, initial: Agent) {
  const locale = useLocale() as Locale;
  const t = useTranslations("agents");
  const notify = useNotify();
  const router = useRouter();
  const [agent, setAgent] = useState(initial);
  const [draft, setDraft] = useState<AgentDraft>(() => fromAgent(initial));
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);

  function update(change: Partial<AgentDraft>) {
    if (savingRef.current) return;
    setDraft((current) => ({ ...current, ...change }));
  }

  function discard() {
    if (savingRef.current) return;
    setDraft(fromAgent(agent));
  }

  async function persist() {
    if (savingRef.current || !canSaveDraft(agent, draft)) return;
    savingRef.current = true;
    setSaving(true);
    try {
      const saved = await patchAgent(orgId, agent, draft, locale);
      setAgent(saved);
      setDraft(fromAgent(saved));
      notify.success(t("saved"));
      router.refresh();
    } catch (err) {
      notify.error(err);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }

  return {
    agent,
    draft,
    errors: draftErrors(draft),
    dirty: isDirty(agent, draft),
    canSave: canSaveDraft(agent, draft),
    saving,
    update,
    discard,
    persist,
  };
}
