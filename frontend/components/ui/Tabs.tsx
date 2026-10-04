"use client";

import { useRef, type KeyboardEvent, type ReactNode } from "react";

import { cn } from "@/lib/cn";

export type TabItem<T extends string> = { id: T; label: ReactNode };

type Props<T extends string> = {
  items: ReadonlyArray<TabItem<T>>;
  value: T;
  onChange: (id: T) => void;
  /** Prefix for tab / panel ids so `aria-controls` can point at the panel. */
  idPrefix: string;
  label: string;
  className?: string;
  size?: "sm" | "md";
};

export function tabPanelId(idPrefix: string, id: string) {
  return `${idPrefix}-panel-${id}`;
}

const STEP: Record<string, number> = { ArrowRight: 1, ArrowLeft: -1 };

/** Index the key moves to, or undefined when the key is not a tab-navigation key. */
function targetIndex(key: string, index: number, count: number): number | undefined {
  if (key === "Home") return 0;
  if (key === "End") return count - 1;
  const step = STEP[key];
  return step === undefined ? undefined : (index + step + count) % count;
}

/**
 * A tab rail with an ink underline that slides between tabs. Arrows (wrapping), Home and End move focus and
 * selection; only the selected tab is in the tab order (WAI-ARIA tabs pattern, automatic activation).
 */
export function Tabs<T extends string>({
  items,
  value,
  onChange,
  idPrefix,
  label,
  className,
  size = "sm",
}: Props<T>) {
  const refs = useRef<Array<HTMLButtonElement | null>>([]);
  function onKeyDown(event: KeyboardEvent, index: number) {
    const next = targetIndex(event.key, index, items.length);
    if (next === undefined) return;
    event.preventDefault();
    refs.current[next]?.focus();
    const item = items[next];
    if (item) onChange(item.id);
  }
  return (
    <div
      role="tablist"
      aria-label={label}
      className={cn(
        // On phones the rail scrolls; the faded right edge says there is more, the end padding lets the last tab clear it.
        "flex gap-0.5 overflow-x-auto max-sm:[mask-image:linear-gradient(to_right,#000_82%,transparent)] max-sm:pr-10",
        className,
      )}
    >
      {items.map(({ id, label: text }, index) => (
        <button
          key={id}
          ref={(el) => {
            refs.current[index] = el;
          }}
          type="button"
          role="tab"
          id={`${idPrefix}-tab-${id}`}
          aria-selected={value === id}
          aria-controls={tabPanelId(idPrefix, id)}
          tabIndex={value === id ? 0 : -1}
          onClick={() => onChange(id)}
          onKeyDown={(event) => onKeyDown(event, index)}
          className={cn(
            "text-ink-3 hover:text-ink aria-selected:text-ink relative -mb-px shrink-0 whitespace-nowrap transition-colors aria-selected:font-semibold",
            "after:bg-ink after:ease-out-expo after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:origin-center after:scale-x-0 after:transition-transform after:duration-300 aria-selected:after:scale-x-100",
            size === "sm" ? "px-2.5 py-2.75 text-[13px]" : "px-3.5 py-3.5 text-sm",
          )}
        >
          {text}
        </button>
      ))}
    </div>
  );
}
