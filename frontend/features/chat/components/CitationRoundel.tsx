import { lineBg, onLine, type Line } from "@/components/metro/lines";
import { cn } from "@/lib/cn";

type Props = {
  n: number;
  label: string;
  line?: Line;
  selected?: boolean;
  onSelect?: (n: number) => void;
  className?: string;
};

/** A numbered roundel in its source line's colour; selecting it opens the exact cited block. */
export function CitationRoundel({ n, label, line = "kb", selected = false, onSelect, className }: Props) {
  const look = cn(
    "tabular inline-grid h-[19px] min-w-[19px] place-items-center rounded-full px-1 text-[10.5px] leading-none font-semibold",
    lineBg[line],
    onLine[line],
    className,
  );
  if (!onSelect) {
    return (
      <span className={look} aria-label={label}>
        {n}
      </span>
    );
  }
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={selected}
      onClick={() => onSelect(n)}
      className={cn(
        look,
        "ease-out-expo mx-0.5 -translate-y-px align-middle transition-transform duration-200 hover:scale-[1.18] focus-visible:scale-[1.18] aria-pressed:scale-[1.18]",
      )}
    >
      {n}
    </button>
  );
}
