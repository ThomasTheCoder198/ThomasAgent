"use client";

import { useEffect, useRef } from "react";

/** How close to the bottom (px) still counts as "following" the stream. */
const FOLLOW_THRESHOLD = 120;

/** Keeps the thread pinned to the newest station while streaming, unless the owner scrolled up to read. */
export function useStickToBottom<T extends HTMLElement>(signal: unknown) {
  const ref = useRef<T>(null);
  const following = useRef(true);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const onScroll = () => {
      following.current = el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_THRESHOLD;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    const el = ref.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [signal]);

  return ref;
}
