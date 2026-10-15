"use client";

import type { LucideIcon } from "lucide-react";
import { useId } from "react";

import { cn } from "@/lib/cn";

import { ToggleGroup, ToggleGroupItem } from "./ToggleGroup";

export type SegmentedOption<T extends string> = { value: T; label: string; icon?: LucideIcon };

type Props<T extends string> = {
  /** Names the group for assistive tech; shown above it unless `hideLegend`. */
  legend: string;
  value: T | undefined;
  options: ReadonlyArray<SegmentedOption<T>>;
  onChange: (value: T) => void;
  disabled?: boolean;
  hideLegend?: boolean;
  className?: string;
};

/**
 * A one-of-N choice drawn as a segmented control. Radix lets a single-select group be cleared by pressing the
 * chosen segment again; a choice that must always hold a value ignores that.
 */
export function SegmentedChoice<T extends string>({
  legend,
  value,
  options,
  onChange,
  disabled,
  hideLegend,
  className,
}: Props<T>) {
  const legendId = useId();
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <span
        id={legendId}
        className={cn("text-muted-foreground text-xs font-medium", hideLegend && "sr-only")}
      >
        {legend}
      </span>
      <ToggleGroup
        type="single"
        aria-labelledby={legendId}
        value={value ?? ""}
        disabled={disabled}
        onValueChange={(next) => {
          if (next) onChange(next as T);
        }}
        className="w-full"
      >
        {options.map(({ value: optionValue, label, icon: Icon }) => (
          <ToggleGroupItem key={optionValue} value={optionValue}>
            {Icon && <Icon aria-hidden />}
            {label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
