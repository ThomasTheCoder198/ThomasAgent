import type { LucideIcon } from "lucide-react";

import { cn } from "@/lib/cn";

export type SegmentedOption<T extends string> = { value: T; label: string; icon?: LucideIcon };

type Props<T extends string> = {
  name: string;
  legend: string;
  value: T | undefined;
  options: ReadonlyArray<SegmentedOption<T>>;
  onChange: (value: T) => void;
  disabled?: boolean;
  /** `sign` sits on the enamel-black sign; `ground` on station white / platform night. */
  tone?: "sign" | "ground";
  hideLegend?: boolean;
};

const tones = {
  sign: {
    legend: "text-sign-ink-2",
    track: "bg-sign-hover",
    option: "text-sign-ink-2 hover:text-sign-ink has-checked:bg-sign-ink has-checked:text-sign",
  },
  ground: {
    legend: "text-ink-2",
    track: "bg-rule-2 border-rule border",
    option:
      "text-ink-2 hover:text-ink has-checked:bg-paper has-checked:text-ink has-checked:shadow-[0_1px_2px_rgb(0_0_0/0.12)]",
  },
} as const;

/** A radio group drawn as a signage switch. */
export function Segmented<T extends string>(props: Props<T>) {
  const { name, legend, value, options, onChange, disabled, tone = "sign", hideLegend } = props;
  const look = tones[tone];
  return (
    <fieldset className="flex flex-col gap-1.5" disabled={disabled}>
      <legend className={cn("mb-1.5 text-xs font-medium", look.legend, hideLegend && "sr-only")}>
        {legend}
      </legend>
      <div className={cn("flex gap-0.5 rounded-lg p-0.5", look.track)}>
        {options.map(({ value: optionValue, label, icon: Icon }) => (
          <label
            key={optionValue}
            className={cn(
              "has-focus-visible:outline-focus flex flex-1 cursor-pointer items-center justify-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs whitespace-nowrap transition-[background-color,color,transform] duration-200 active:scale-[0.96] has-focus-visible:outline-2",
              look.option,
            )}
          >
            <input
              type="radio"
              name={name}
              value={optionValue}
              checked={value === optionValue}
              onChange={() => onChange(optionValue)}
              className="sr-only"
              aria-label={label}
            />
            {Icon && <Icon aria-hidden className="size-3.5" />}
            <span aria-hidden>{label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
