"use client";

import { useCallback, useState } from "react";

import type { EvidenceTab } from "./evidence/EvidencePanel";

type EvidenceState = {
  open: boolean;
  tab: EvidenceTab;
  messageId: string | undefined;
  n: number | undefined;
};

/** Which citation the evidence panel shows, for which message, on which tab. */
export function useEvidence(initiallyOpen: boolean) {
  const [state, setState] = useState<EvidenceState>({
    open: initiallyOpen,
    tab: "evidence",
    messageId: undefined,
    n: undefined,
  });
  const cite = useCallback(
    (messageId: string, n: number) => setState({ open: true, tab: "evidence", messageId, n }),
    [],
  );
  const inspect = useCallback(
    (messageId?: string) =>
      setState((s) => ({ ...s, open: true, tab: "inspector", messageId: messageId ?? s.messageId })),
    [],
  );
  const setTab = useCallback((tab: EvidenceTab) => setState((s) => ({ ...s, tab })), []);
  const close = useCallback(() => setState((s) => ({ ...s, open: false })), []);
  const selectInMessage = useCallback((n: number) => setState((s) => ({ ...s, tab: "evidence", n })), []);
  return { ...state, cite, inspect, setTab, close, selectInMessage };
}
