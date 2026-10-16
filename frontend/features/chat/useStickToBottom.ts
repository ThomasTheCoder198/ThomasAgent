"use client";

import { useCallback, useEffect, useRef } from "react";

/** How close to the bottom (px) still counts as "following" the stream. */
const FOLLOW_THRESHOLD = 120;
/** How long after a wheel, touch, key or scrollbar press a scroll still counts as the owner's own. */
const USER_SCROLL_WINDOW_MS = 400;
const SCROLL_KEYS = new Set(["ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", " "]);

/**
 * Keeps the thread pinned to the newest station while streaming, unless the owner scrolled up to read.
 * Returns a callback ref: the scroller only exists once the first message does (the hero has none), so the
 * listeners must attach whenever the element appears, not just at mount. Content also changes size after a
 * message changes (approval cards, tables, the run folding into its summary), so the content's size is watched too;
 * the scroller disables scroll anchoring, which would otherwise move the view up when the run folds.
 *
 * Only the owner's own input may stop following. When a message re-renders, its height can dip for a frame and the
 * browser clamps scrollTop; that scroll event arrives after the content has grown back and would otherwise read as
 * "scrolled up" (approving a tool used to leave the thread stranded above its end).
 */
export function useStickToBottom<T extends HTMLElement>(signal: unknown) {
  const element = useRef<T | null>(null);
  const following = useRef(true);

  const attach = useCallback((node: T | null) => {
    element.current = node;
    if (!node) return;
    let userUntil = 0;
    const markUser = () => {
      userUntil = performance.now() + USER_SCROLL_WINDOW_MS;
    };
    const onKey = (event: KeyboardEvent) => SCROLL_KEYS.has(event.key) && markUser();
    // A press on the scroller itself (not its content) is a press on its scrollbar.
    const onPointer = (event: PointerEvent) => event.target === node && markUser();
    const pin = () => {
      if (following.current) node.scrollTop = node.scrollHeight;
    };
    const onScroll = () => {
      const nearBottom = node.scrollHeight - node.scrollTop - node.clientHeight < FOLLOW_THRESHOLD;
      if (nearBottom) following.current = true;
      else if (performance.now() < userUntil) following.current = false;
      else pin();
    };
    const listeners = [
      ["scroll", onScroll],
      ["wheel", markUser],
      ["touchmove", markUser],
      ["keydown", onKey],
      ["pointerdown", onPointer],
    ] as const;
    for (const [name, listener] of listeners)
      node.addEventListener(name, listener as EventListener, { passive: true });
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(pin);
    if (resize) {
      resize.observe(node);
      for (const child of node.children) resize.observe(child);
    }
    return () => {
      for (const [name, listener] of listeners) node.removeEventListener(name, listener as EventListener);
      resize?.disconnect();
    };
  }, []);

  useEffect(() => {
    const el = element.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [signal]);

  return attach;
}
