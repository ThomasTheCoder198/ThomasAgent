"use client";

import { useCallback, useEffect, useRef } from "react";

/** How close to the bottom (px) still counts as "following" the stream. */
const FOLLOW_THRESHOLD = 120;

/**
 * Keeps the thread pinned to the newest station while streaming, unless the owner scrolled up to read.
 * Returns a callback ref: the scroller only exists once the first message does (the hero has none), so the
 * scroll listener must attach whenever the element appears, not just at mount. Content also changes size after a
 * message changes (approval cards, tables, the run folding into its summary), so the content's size is watched too;
 * the scroller disables scroll anchoring, which would otherwise move the view up when the run folds.
 */
export function useStickToBottom<T extends HTMLElement>(signal: unknown) {
  const element = useRef<T | null>(null);
  const following = useRef(true);

  const attach = useCallback((node: T | null) => {
    element.current = node;
    if (!node) return;
    const pin = () => {
      if (following.current) node.scrollTop = node.scrollHeight;
    };
    const onScroll = () => {
      following.current = node.scrollHeight - node.scrollTop - node.clientHeight < FOLLOW_THRESHOLD;
    };
    node.addEventListener("scroll", onScroll, { passive: true });
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(pin);
    if (resize) {
      resize.observe(node);
      for (const child of node.children) resize.observe(child);
    }
    return () => {
      node.removeEventListener("scroll", onScroll);
      resize?.disconnect();
    };
  }, []);

  useEffect(() => {
    const el = element.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [signal]);

  return attach;
}
