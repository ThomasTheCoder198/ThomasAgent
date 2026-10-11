"use client";

import { Switch as SwitchPrimitive } from "radix-ui";
import { useId } from "react";

/** A labelled on/off control; the hint explains the consequence, not the mechanism. */
export function Switch({
  label,
  hint,
  checked,
  onCheckedChange,
  disabled,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  const hintId = useId();
  return (
    <label className="min-h-touch flex items-start justify-between gap-4 text-sm font-medium">
      <span>
        {label}
        {hint && (
          <span id={hintId} className="text-ink-2 mt-0.5 block text-[13px] leading-relaxed font-normal">
            {hint}
          </span>
        )}
      </span>
      <SwitchPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-label={label}
        aria-describedby={hint ? hintId : undefined}
        className="bg-rule-strong data-[state=checked]:bg-ink relative mt-0.5 inline-flex h-6 w-10 shrink-0 items-center rounded-full transition-colors duration-200 disabled:opacity-60"
      >
        <SwitchPrimitive.Thumb className="bg-paper ease-out-expo block size-5 translate-x-0.5 rounded-full transition-transform duration-200 data-[state=checked]:translate-x-[1.125rem]" />
      </SwitchPrimitive.Root>
    </label>
  );
}
