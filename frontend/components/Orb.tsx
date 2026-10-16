"use client";

import { useTheme } from "next-themes";
import { useEffect, useRef } from "react";
import type { OrbSize, OrbState } from "thinking-orbs";
import { MODE_FRAMES, paintFrame, resolvePreset } from "thinking-orbs/engine";

import { useGentleMotion } from "./MotionProvider";

type Props = {
  state: OrbState;
  size: OrbSize;
  /** `#rgb` or `#rrggbb`; omitted, the orb is stock grayscale. */
  color?: string;
  className?: string;
};

const MAX_DPR = 2;
const MS_PER_SECOND = 1000;
const HEX = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i;
const SHORT_HEX_LENGTH = 3;
const HEX_RADIX = 16;

function parseTint(color: string | undefined) {
  const hex = color?.trim().match(HEX)?.[1];
  if (!hex) return undefined;
  const pairs = hex.length === SHORT_HEX_LENGTH ? [...hex].map((c) => c + c) : (hex.match(/../g) ?? []);
  const [r = 0, g = 0, b = 0] = pairs.map((pair) => parseInt(pair, HEX_RADIX));
  return { r, g, b };
}

/**
 * A Thinking Orb drawn with the library's own geometry and ink (`thinking-orbs/engine`), minus one behaviour:
 * `<ThinkingOrb>` freezes on a still frame whenever the OS reports reduced motion, and `paused`/`speed` cannot
 * undo that. The owner keeps "animation effects" off in Windows and still wants living orbs (decision
 * 2026-10-16), so under reduced motion this one keeps running at 0.6x. It is decorative (`aria-hidden`): the text
 * beside it carries the meaning. The loop stops while the canvas is off screen or the tab is hidden.
 */
export function Orb({ state, size, color, className = "block" }: Props) {
  const ref = useRef<HTMLCanvasElement>(null);
  const { orbSpeed } = useGentleMotion();
  const dark = useTheme().resolvedTheme === "dark";

  useEffect(() => {
    const canvas = ref.current;
    // jsdom has no canvas context.
    const ctx = canvas?.getContext?.("2d");
    if (!canvas || !ctx) return;
    const dpr = Math.min(MAX_DPR, window.devicePixelRatio || 1);
    canvas.width = Math.round(size * dpr);
    canvas.height = Math.round(size * dpr);
    const { mode, speed, opts } = resolvePreset(state, size);
    const geometry = MODE_FRAMES[mode];
    const tint = parseTint(color);
    const draw = () => {
      const seconds = (performance.now() / MS_PER_SECOND) * speed * orbSpeed;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, size, size);
      paintFrame(ctx, geometry(size, seconds, opts), dark, tint);
    };

    let raf = 0;
    let running = false;
    const loop = () => {
      draw();
      if (running) raf = requestAnimationFrame(loop);
    };
    const start = () => {
      if (running) return;
      running = true;
      raf = requestAnimationFrame(loop);
    };
    const stop = () => {
      running = false;
      cancelAnimationFrame(raf);
    };

    draw();
    let visible = true;
    const observer =
      typeof IntersectionObserver === "undefined"
        ? null
        : new IntersectionObserver((entries) => {
            visible = entries.at(-1)?.isIntersecting ?? visible;
            if (visible && document.visibilityState !== "hidden") start();
            else stop();
          });
    observer?.observe(canvas);
    const onVisibility = () => {
      if (document.visibilityState === "hidden") stop();
      else if (visible) start();
    };
    document.addEventListener("visibilitychange", onVisibility);
    if (!observer) start();
    return () => {
      stop();
      observer?.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [state, size, color, dark, orbSpeed]);

  return <canvas ref={ref} aria-hidden style={{ width: size, height: size }} className={className} />;
}
