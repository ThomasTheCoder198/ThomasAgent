import type { LucideIcon } from "lucide-react";

export type SegmentedOption<T extends string> = { value: T; label: string; icon?: LucideIcon };

type Props<T extends string> = {
  name: string;
  legend: string;
  value: T | undefined;
  options: ReadonlyArray<SegmentedOption<T>>;
  onChange: (value: T) => void;
  disabled?: boolean;
};

/** A radio group drawn as a signage switch; sits on the enamel-black sign surface. */
export function Segmented<T extends string>({ name, legend, value, options, onChange, disabled }: Props<T>) {
  return (
    <fieldset className="flex flex-col gap-1.5" disabled={disabled}>
      <legend className="text-sign-ink-2 mb-1.5 text-xs font-medium">{legend}</legend>
      <div className="bg-sign-hover flex gap-0.5 rounded-lg p-0.5">
        {options.map(({ value: optionValue, label, icon: Icon }) => (
          <label
            key={optionValue}
            className="text-sign-ink-2 hover:text-sign-ink has-checked:bg-sign-ink has-checked:text-sign has-focus-visible:outline-focus flex flex-1 cursor-pointer items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs transition-colors has-focus-visible:outline-2"
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
